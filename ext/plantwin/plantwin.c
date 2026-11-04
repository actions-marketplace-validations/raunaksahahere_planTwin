/*
 * plantwin: CI-only planner extension for PlanTwin.
 *
 * What it does
 *   Installs a chained get_relation_info_hook. When plantwin.enabled is on, the
 *   planner's size estimates for a table and its indexes are replaced with the
 *   values stored in pg_class (relpages, reltuples, relallvisible), which are
 *   written there by pg_restore_relation_stats(). The planner normally derives
 *   the table size from the physical block count, so an empty CI database would
 *   otherwise estimate 0 rows. See docs/design-note-planner-hook.md.
 *
 * Safety
 *   - _PG_init() refuses to load unless plantwin.ci_marker is set to a non-empty
 *     value. This also stops a shared_preload_libraries entry from starting a
 *     production server: the postmaster fails at startup instead.
 *   - plantwin.enabled defaults to off and can only be set by superusers.
 *   - The hook never writes to disk or to any catalog. It only changes the
 *     planner's in-memory RelOptInfo / IndexOptInfo for the current query.
 *
 * Memory and hook discipline
 *   - Catalog tuples come from SearchSysCache and are released with
 *     ReleaseSysCache() before returning. Values are copied out first.
 *   - The previous hook is called first, so this extension's values win and
 *     other extensions in the chain keep their behaviour.
 */
#include "postgres.h"

#include <math.h>

#include "access/htup_details.h"
#include "access/itup.h"
#include "access/nbtree.h"
#include "catalog/pg_am_d.h"
#include "catalog/pg_class.h"
#include "catalog/pg_statistic.h"
#include "storage/bufpage.h"
#include "storage/itemid.h"
#include "nodes/pathnodes.h"
#include "optimizer/plancat.h"
#include "utils/guc.h"
#include "utils/syscache.h"

PG_MODULE_MAGIC;

void		_PG_init(void);

static bool plantwin_enabled = false;
static char *plantwin_ci_marker = NULL;
static get_relation_info_hook_type prev_get_relation_info_hook = NULL;

/*
 * Estimate the B-tree height (the root's level, as returned by the
 * btree tree-height callback) from imported sizes.
 *
 * The planner reads the real height from the metapage (_bt_getrootheight). An
 * empty CI index has no root, so the planner would use height 0 and make index
 * scans look cheaper. We estimate it instead:
 *     leaves     = pages - 1                         (one metapage)
 *     key width  = sum of pg_statistic.stawidth for the key columns
 *                  (8 bytes for an expression column or a missing stat)
 *     itemsize   = MAXALIGN(IndexTuple header + key width) + line pointer
 *     fanout     = floor(usable page bytes * fill / itemsize), fill = 70%
 *                  (BTREE_NONLEAF_FILLFACTOR: internal pages are built to 70%)
 *     level      = ceil(log(leaves) / log(fanout)) for leaves > 1, else 0
 * Constants come from the PG18 headers and nbtsort.c, not from fitting data.
 * Checked on TPC-H SF1 (8/8 indexes) and JOB (42/44). See
 * docs/decisions/0002-btree-height-estimate.md for the two misses.
 */
#define PLANTWIN_NONLEAF_FILL 0.70

static int32
index_key_width(Oid tableoid, IndexOptInfo *index)
{
	int32		width = 0;
	int			i;

	for (i = 0; i < index->nkeycolumns; i++)
	{
		AttrNumber	attnum = index->indexkeys[i];
		HeapTuple	tp;
		int32		w = 0;

		if (attnum > 0)
		{
			tp = SearchSysCache3(STATRELATTINH, ObjectIdGetDatum(tableoid),
								 Int16GetDatum(attnum), BoolGetDatum(false));
			if (HeapTupleIsValid(tp))
			{
				w = ((Form_pg_statistic) GETSTRUCT(tp))->stawidth;
				ReleaseSysCache(tp);
			}
		}
		width += (w > 0) ? w : 8;
	}
	return width;
}

static int
estimate_btree_height(double index_pages, Oid tableoid, IndexOptInfo *index)
{
	double		leaves = index_pages - 1.0;
	double		usable;
	double		itemsize;
	double		fanout;

	if (leaves <= 1.0)
		return 0;

	itemsize = MAXALIGN(sizeof(IndexTupleData) + index_key_width(tableoid, index))
		+ sizeof(ItemIdData);
	usable = (BLCKSZ - SizeOfPageHeaderData - MAXALIGN(sizeof(BTPageOpaqueData)))
		* PLANTWIN_NONLEAF_FILL;
	fanout = floor(usable / itemsize);
	if (fanout < 2.0)
		fanout = 2.0;
	return (int) ceil(log(leaves) / log(fanout));
}

/*
 * Read imported size stats for one pg_class entry.
 * Returns false when the entry has no imported stats (relpages <= 0 or
 * reltuples < 0, which PG14+ uses for "never analyzed").
 */
static bool
read_imported_size(Oid relid, int32 *relpages, float4 *reltuples, int32 *relallvisible)
{
	HeapTuple	tp;
	Form_pg_class classform;
	bool		found = false;

	tp = SearchSysCache1(RELOID, ObjectIdGetDatum(relid));
	if (!HeapTupleIsValid(tp))
		return false;

	classform = (Form_pg_class) GETSTRUCT(tp);
	if (classform->relpages > 0 && classform->reltuples >= 0)
	{
		*relpages = classform->relpages;
		*reltuples = classform->reltuples;
		*relallvisible = classform->relallvisible;
		found = true;
	}
	ReleaseSysCache(tp);
	return found;
}

static void
plantwin_get_relation_info(PlannerInfo *root, Oid relationObjectId,
						   bool inhparent, RelOptInfo *rel)
{
	ListCell   *lc;
	int32		relpages;
	float4		reltuples;
	int32		relallvisible;

	if (prev_get_relation_info_hook)
		prev_get_relation_info_hook(root, relationObjectId, inhparent, rel);

	if (!plantwin_enabled)
		return;

	/* Partitioned tables have no storage, so pg_class.relpages is 0 and is skipped here. */
	if (read_imported_size(relationObjectId, &relpages, &reltuples, &relallvisible))
	{
		rel->pages = (BlockNumber) relpages;
		rel->tuples = (double) reltuples;
		rel->allvisfrac = (double) relallvisible / (double) relpages;
		if (rel->allvisfrac > 1.0)
			rel->allvisfrac = 1.0;
		elog(DEBUG1, "plantwin: relation %u: pages=%u tuples=%.0f",
			 relationObjectId, rel->pages, rel->tuples);
	}

	foreach(lc, rel->indexlist)
	{
		IndexOptInfo *index = (IndexOptInfo *) lfirst(lc);
		int32		ipages;
		float4		ituples;
		int32		ivis;

		if (read_imported_size(index->indexoid, &ipages, &ituples, &ivis))
		{
			index->pages = (BlockNumber) ipages;
			/*
			 * Mirror plancat.c: a non-partial index takes the table's tuple
			 * estimate; a partial index uses its own, capped at the table's.
			 */
			if (index->indpred == NIL)
				index->tuples = rel->tuples;
			else
				index->tuples = Min((double) ituples, rel->tuples);
			if (index->relam == BTREE_AM_OID)
				index->tree_height = estimate_btree_height((double) ipages, relationObjectId, index);
			elog(DEBUG1, "plantwin: index %u: pages=%u tuples=%.0f",
				 index->indexoid, index->pages, index->tuples);
		}
	}
}

void
_PG_init(void)
{
	const char *marker;

	/*
	 * Check the marker before defining any GUC. If we defined plantwin.ci_marker
	 * and then raised an error, a second LOAD in the same session would fail with
	 * "attempt to redefine parameter". GetConfigOption() with missing_ok returns
	 * the value of a placeholder set by SET, or NULL if none was set.
	 */
	marker = GetConfigOption("plantwin.ci_marker", true, false);
	if (marker == NULL || marker[0] == '\0')
		ereport(ERROR,
				(errcode(ERRCODE_FEATURE_NOT_SUPPORTED),
				 errmsg("plantwin refuses to load outside CI"),
				 errdetail("plantwin.ci_marker is not set."),
				 errhint("This extension changes planner estimates and must never run in production. "
						 "Set plantwin.ci_marker in a CI session before LOAD.")));

	DefineCustomStringVariable("plantwin.ci_marker",
							   "Marks a CI session. plantwin refuses to load unless this is set.",
							   NULL,
							   &plantwin_ci_marker,
							   "",
							   PGC_SUSET,
							   0,
							   NULL, NULL, NULL);

	DefineCustomBoolVariable("plantwin.enabled",
							 "Use imported pg_class sizes for planner estimates.",
							 NULL,
							 &plantwin_enabled,
							 false,
							 PGC_SUSET,
							 0,
							 NULL, NULL, NULL);

	prev_get_relation_info_hook = get_relation_info_hook;
	get_relation_info_hook = plantwin_get_relation_info;
}
