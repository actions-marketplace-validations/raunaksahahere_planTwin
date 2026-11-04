# plantwin-postgres: official PostgreSQL 18 with the plantwin extension installed.
# The extension is NOT preloaded. It loads only in CI sessions that set plantwin.ci_marker.
# Build locally with:  docker build -t plantwin-postgres:18 .
# Test target (runs pg_regress):  docker build --target test -t plantwin-postgres:18-test .

FROM postgres:18 AS build
RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential postgresql-server-dev-18 \
 && rm -rf /var/lib/apt/lists/*
COPY ext/plantwin /src/plantwin
WORKDIR /src/plantwin
RUN make USE_PGXS=1 && make USE_PGXS=1 install

FROM build AS test
RUN chown -R postgres:postgres /src/plantwin
USER postgres
RUN ./run_regress.sh

FROM postgres:18 AS final
LABEL org.opencontainers.image.title="plantwin-postgres" \
      org.opencontainers.image.description="PostgreSQL 18 with the plantwin CI-only planner extension (not preloaded)" \
      org.opencontainers.image.licenses="PostgreSQL AND Apache-2.0"
COPY --from=build /usr/lib/postgresql/18/lib/plantwin.so /usr/lib/postgresql/18/lib/
COPY --from=build /usr/share/postgresql/18/extension/plantwin* /usr/share/postgresql/18/extension/
