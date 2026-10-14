# piiscrub — common developer tasks.
#
# Datasets are NOT committed to this repository (see DATASET_LICENSES.md).
# `make download` fetches the corpora into testdata/external/, which .gitignore
# excludes — keeping dataset terms from reaching this source tree. Only corpora
# whose licences permit commercial use are fetched; the restricted ones are
# skipped and can be inspected individually with `make info NAME=<dataset>`.

GO    ?= go
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

.PHONY: help download list info dry-run benchmark test test-tools build vet fmt \
        corpus corpus-convert corpus-anonymize corpus-clean clean-data

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
	@echo "  make test            run the library test suite"
	@echo "  make test-tools      run the corpusbench tool test suite"
	@echo "  make build           build every package"
	@echo "  make vet             run go vet"
	@echo "  make fmt             fail if any file is not gofmt-clean"
	@echo "  make clean-data      delete downloaded corpora (testdata/external)"

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

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

fmt:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; fi; \
	echo "gofmt: clean"

# The tool lives in its own module so parquet-go never reaches the library's
# dependency list; every target that uses it runs inside that module.
corpus-convert:
	cd tools/corpusbench && $(GO) run . convert nemotron-pii "$(DATA_ROOT)" $(LIMIT)
	cd tools/corpusbench && $(GO) run . convert gretel-pii-finance "$(DATA_ROOT)" $(LIMIT)

corpus: corpus-convert
	./scripts/benchmark.sh "$(DATA_ROOT)/nemotron-pii/converted"
	./scripts/benchmark.sh "$(DATA_ROOT)/gretel-pii-finance/converted"

corpus-anonymize:
	$(GO) run ./cmd/corpusanon "$(DATA_ROOT)/nemotron-pii/converted" "$(DATA_ROOT)/gretel-pii-finance/converted"

corpus-clean:
	rm -rf "$(DATA_ROOT)/nemotron-pii/converted" "$(DATA_ROOT)/gretel-pii-finance/converted"
	@echo "removed converted datasets — 'make corpus-convert' regenerates them"

test-tools:
	cd tools/corpusbench && $(GO) test ./...

clean-data:
	rm -rf testdata/external
	@echo "removed testdata/external — run 'make download' to refetch"
