// Command driver runs the PlanTwin rules over one corpus case.
//
// usage: go run ./bench/corpus/driver -base <dir> -head <dir> -estimated <file.json> -out <file.json>
//
// <base> holds qNN.json plans. <head> holds qNN.json plans, or qNN.err when the query no longer plans.
// <estimated> is a JSON array of relation names whose stats are estimated.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/raunaksahahere/plantwin/internal/plan"
	"github.com/raunaksahahere/plantwin/internal/rules"
)

type result struct {
	Query    string          `json:"query"`
	HeadErr  string          `json:"head_error,omitempty"`
	Findings []rules.Finding `json:"findings"`
}

func main() {
	base := flag.String("base", "", "directory with base plans")
	head := flag.String("head", "", "directory with head plans or .err files")
	est := flag.String("estimated", "", "JSON array of estimated relation names")
	tableRows := flag.String("table-rows", "", "JSON object: table name -> row count (from the stats bundle)")
	out := flag.String("out", "", "output JSON file")
	flag.Parse()
	if *base == "" || *head == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "driver: -base, -head and -out are required")
		os.Exit(2)
	}

	cfg := rules.DefaultConfig()
	cfg.EstimatedRelns = map[string]bool{}
	if *tableRows != "" {
		data, err := os.ReadFile(*tableRows)
		exitOn(err)
		var rows map[string]*float64
		exitOn(json.Unmarshal(data, &rows))
		cfg.TableRows = map[string]float64{}
		for name, v := range rows {
			if v != nil {
				cfg.TableRows[name] = *v
			}
		}
	}
	if *est != "" {
		data, err := os.ReadFile(*est)
		exitOn(err)
		var names []string
		exitOn(json.Unmarshal(data, &names))
		for _, n := range names {
			cfg.EstimatedRelns[n] = true
		}
	}

	entries, err := filepath.Glob(filepath.Join(*base, "*.json"))
	exitOn(err)
	sort.Strings(entries)
	var results []result
	for _, path := range entries {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		baseRoot := load(path)
		in := rules.Input{Query: name, Base: baseRoot}
		res := result{Query: name}
		if headPath := filepath.Join(*head, name+".json"); exists(headPath) {
			in.Head = load(headPath)
		} else if errPath := filepath.Join(*head, name+".err"); exists(errPath) {
			msg, err := os.ReadFile(errPath)
			exitOn(err)
			in.HeadErr = strings.TrimSpace(string(msg))
			res.HeadErr = in.HeadErr
		} else {
			exitOn(fmt.Errorf("no head plan or error for %s", name))
		}
		res.Findings = rules.Analyse(in, cfg)
		if res.Findings == nil {
			res.Findings = []rules.Finding{}
		}
		results = append(results, res)
	}
	data, err := json.MarshalIndent(results, "", "  ")
	exitOn(err)
	exitOn(os.WriteFile(*out, append(data, '\n'), 0o644))
	fmt.Printf("analysed %d queries -> %s\n", len(results), *out)
}

func load(path string) plan.Node {
	data, err := os.ReadFile(path)
	exitOn(err)
	n, err := plan.Parse(data)
	exitOn(err)
	return n
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "driver:", err)
		os.Exit(1)
	}
}
