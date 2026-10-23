# piiscrub — common developer tasks.
#
# Datasets are NOT committed to this repository (see DATASET_LICENSES.md).
# `make download` fetches the corpora into testdata/external/, which .gitignore
# excludes — keeping dataset terms from reaching this source tree. Only corpora
# whose licences permit commercial use are fetched; the restricted ones are
# skipped and can be inspected individually with `make info NAME=<dataset>`.

GO    ?= go
# From GOROOT, not PATH: this environment's PATH does not carry the Go tools, and
# a bare `gofmt` then fails inside the recipe while the empty result reads as
# "clean". A format gate that silently passes when it cannot run is worse than
# none.
GOFMT ?= $(shell $(GO) env GOROOT)/bin/gofmt
FETCH := ./scripts/fetch-dataset.sh

# Corpora live outside the tree (see DATASET_LICENSES.md). DATAPROTECT_DATA_DIR
# is the same variable scripts/fetch-dataset.sh honours, so a fetch and a
# conversion cannot disagree about where the data is.
#
# ABSOLUTE, because every target using it first runs inside tools/corpusbench
# (the tool is its own module). A relative root would then resolve against
# tools/corpusbench/ instead of the repository, and the conversion would fail
# to find a corpus that is sitting right there on disk.
DATA_ROOT := $(abspath $(or $(DATAPROTECT_DATA_DIR),testdata/external))

# How many records to convert per corpus. The bound is recorded in each
# converted dataset's schema.json, so a sampled report cannot be mistaken for a
# full-corpus one. 0 means every record.
LIMIT ?= 2000

.PHONY: help download list info dry-run benchmark test test-race test-tools build vet fmt \
        corpus corpus-convert corpus-anonymize corpus-clean corpus-nlp clean-data presidio presidio-fetch \
        build-sidecar test-sidecar vet-sidecar docs-sync test-ner docs-check

help:
	@echo "piiscrub targets:"
	@echo "  make download        fetch the commercially-usable datasets (~450 MB)"
	@echo "  make list            list known datasets and their licences"
	@echo "  make info NAME=...   licence and obligations for one dataset"
	@echo "  make dry-run         show what 'make download' would fetch, without fetching"
	@echo "  make benchmark       score the golden seed set and print the report"
	@echo "  make corpus-convert  convert both corpora to golden datasets (LIMIT=$(LIMIT))"
	@echo "  make corpus          convert, then score each corpus and print the report"
	@echo "  make corpus-anonymize run Scrubber.Anonymize over every converted case (contract check)"
	@echo "  make corpus-clean    delete the converted datasets (data is gitignored)"
	@echo "  make corpus-nlp      score the corpora with the NLP sidecar (starts a sidecar, then stops it)"
	@echo "  make test            run the library test suite"
	@echo "  make test-race       run the library test suite under the race detector"
	@echo "  make test-tools      run the corpusbench tool test suite"
	@echo "  make build           build every package"
	@echo "  make vet             run go vet"
	@echo "  make fmt             fail if any file is not gofmt-clean"
	@echo "  make docs-check      fail if README.md and the usage doc disagree with the seed set"
	@echo "  make build-sidecar   build the sidecar module (separate Go module)"
	@echo "  make test-sidecar    run the sidecar module test suite"
	@echo "  make vet-sidecar     run go vet in the sidecar module"
	@echo "  make docs-sync       copy SPEC.md and AGENTS.md into their docs/ bundle copies"
	@echo "  make test-ner        run the NER accuracy gate (starts a sidecar, then stops it)"
	@echo "  make clean-data      delete downloaded corpora (testdata/external)"
	@echo "  make presidio        score Presidio's in-scope recognizer test cases (MIT, not committed)"

download:
	$(FETCH) fetch-all

list:
	$(FETCH) list

info:
	@test -n "$(NAME)" || { echo "usage: make info NAME=<dataset>"; exit 1; }
	$(FETCH) info $(NAME)

dry-run:
	$(FETCH) fetch-all --dry-run

benchmark:
	./scripts/benchmark.sh

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

fmt:
	@test -x "$(GOFMT)" || { echo "gofmt not found at $(GOFMT)"; exit 1; }
	@unformatted="$$($(GOFMT) -l .)"; \
	if [ -n "$$unformatted" ]; then echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; fi; \
	echo "gofmt: clean"

docs-check:
	python3 scripts/docs_check.py

# The tool lives in its own module so parquet-go never reaches the library's
# dependency list; every target that uses it runs inside that module.
corpus-convert:
	cd tools/corpusbench && $(GO) run . convert nemotron-pii "$(DATA_ROOT)" $(LIMIT)
	cd tools/corpusbench && $(GO) run . convert gretel-pii-finance "$(DATA_ROOT)" $(LIMIT)

corpus: corpus-convert
	./scripts/benchmark.sh "$(DATA_ROOT)/nemotron-pii/converted"
	./scripts/benchmark.sh "$(DATA_ROOT)/gretel-pii-finance/converted"

# Score the corpora WITH the NLP sidecar, for an apples-to-apples comparison
# with the regex-only score that 'make corpus' prints. The sidecar is started
# and stopped by the target, and the port must be free: a stale sidecar would
# answer with old code.
#
# NER_LIMIT caps how many cases are scored. The sidecar costs about 20 ms per
# text, so the full 2000-case corpora take about 40 seconds each; the default
# here is a sample so the target is usable in a normal shell. The figure
# reported is the sample's, and the schema sidecar records the sample size, so
# a sample can never be mistaken for a full-corpus score.
NER_LIMIT ?= 400
corpus-nlp: build-sidecar
	@if curl -sf $(NER_URL)/healthz >/dev/null 2>&1; then \
		echo "a sidecar is already listening on $(NER_URL); stop it first." >&2; \
		exit 1; \
	fi
	@cd sidecar && $(GO) build -o "$(CURDIR)/$(NER_BIN)" ./cmd/piiscrub-sidecar
	@"$(CURDIR)/$(NER_BIN)" -addr $(NER_ADDR) & echo $$! > .tmp-piiscrub-sidecar.pid
	@for i in $$(seq 1 50); do \
		if curl -sf $(NER_URL)/healthz >/dev/null 2>&1; then break; fi; \
		sleep 0.2; \
	done
	@PII_SCRUB_NLP_URL=$(NER_URL) ./scripts/benchmark.sh "$(DATA_ROOT)/nemotron-pii/converted" $(NER_LIMIT) ; \
	status=$$?; \
	PII_SCRUB_NLP_URL=$(NER_URL) ./scripts/benchmark.sh "$(DATA_ROOT)/gretel-pii-finance/converted" $(NER_LIMIT) ; \
	kill $$(cat .tmp-piiscrub-sidecar.pid) 2>/dev/null || true; \
	rm -f $(NER_BIN) .tmp-piiscrub-sidecar.pid; \
	exit $$status


corpus-anonymize:
	$(GO) run ./cmd/corpusanon "$(DATA_ROOT)/nemotron-pii/converted" "$(DATA_ROOT)/gretel-pii-finance/converted"

corpus-clean:
	rm -rf "$(DATA_ROOT)/nemotron-pii/converted" "$(DATA_ROOT)/gretel-pii-finance/converted"
	@echo "removed converted datasets — 'make corpus-convert' regenerates them"

test-tools:
	cd tools/corpusbench && $(GO) test ./...

# Presidio's in-scope recognizer test cases. Scored apart from the CI gate: the
# gate stays on testdata/golden. The clone and the converted cases are under
# DATA_ROOT, which is gitignored, so the extracted strings are not committed.
PRESIDIO_REV := 2523c7b74a469270c5c78bb253f140eafca21e31
PRESIDIO_SRC := $(DATA_ROOT)/presidio/src

presidio-fetch:
	@mkdir -p "$(DATA_ROOT)/presidio"
	@if [ ! -d "$(PRESIDIO_SRC)/.git" ]; then git clone --quiet https://github.com/microsoft/presidio.git "$(PRESIDIO_SRC)"; fi
	git -C "$(PRESIDIO_SRC)" checkout --quiet $(PRESIDIO_REV)
	@test "$$(git -C "$(PRESIDIO_SRC)" rev-parse HEAD)" = "$(PRESIDIO_REV)" || (echo "presidio: clone is not at $(PRESIDIO_REV)" >&2; exit 1)

presidio: presidio-fetch
	python3 -I scripts/presidio_extract.py "$(PRESIDIO_SRC)" "$(DATA_ROOT)/presidio" $(PRESIDIO_REV)
	@for d in "$(DATA_ROOT)"/presidio/test_*_recognizer/converted; do echo; ./scripts/benchmark.sh "$$d" || exit 1; done

clean-data:
	rm -rf testdata/external
	@echo "removed testdata/external — run 'make download' to refetch"

build-sidecar:
	cd sidecar && $(GO) build ./...

test-sidecar:
	cd sidecar && $(GO) test ./...

vet-sidecar:
	cd sidecar && $(GO) vet ./...

# The NER accuracy gate. It needs a running sidecar, so the target starts one,
# waits for it to answer /healthz, runs the gate and stops it again.
#
# The gate measures the detector against testdata/golden/ner. Its FIRST
# evaluation on text the decoder had never seen gave typed P=0.5333 R=0.6154
# and span-level P=0.8667 R=1.0000. The bounds in ner_accuracy_test.go sit
# below the development-set baseline, so a regression fails.
NER_URL ?= http://127.0.0.1:8787
NER_ADDR ?= 127.0.0.1:8787
NER_BIN := .tmp-piiscrub-sidecar

test-ner: build-sidecar
	@cd sidecar && $(GO) build -o "$(CURDIR)/$(NER_BIN)" ./cmd/piiscrub-sidecar
	@if curl -sf $(NER_URL)/healthz >/dev/null 2>&1; then \
		echo "a sidecar is already listening on $(NER_URL); stop it first." >&2; \
		echo "a stale sidecar would answer the gate with OLD code, which is worse" >&2; \
		echo "than no sidecar at all." >&2; \
		exit 1; \
	fi
	@"$(CURDIR)/$(NER_BIN)" -addr $(NER_ADDR) & echo $$! > .tmp-piiscrub-sidecar.pid
	@for i in $$(seq 1 50); do \
		if curl -sf $(NER_URL)/healthz >/dev/null 2>&1; then break; fi; \
		sleep 0.2; \
	done
	@PII_SCRUB_NLP_URL=$(NER_URL) PII_SCRUB_NLP_REQUIRE=1 $(GO) test -count=1 -tags integration -run TestNER -v . ; \
	status=$$?; \
	kill $$(cat .tmp-piiscrub-sidecar.pid) 2>/dev/null || true; \
	rm -f $(NER_BIN) .tmp-piiscrub-sidecar.pid; \
	exit $$status



# The docs/ bundle holds copies of SPEC.md and AGENTS.md with a YAML frontmatter
# header. docs/concepts/piiscrub-usage.md is a condensed document, not a copy,
# so it is maintained by hand and this target does not touch it.
docs-sync:
	python3 scripts/sync-docs.py
