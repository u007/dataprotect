# piiscrub — common developer tasks.
#
# Datasets are NOT committed to this repository (see DATASET_LICENSES.md).
# `make download` fetches the corpora into testdata/external/, which .gitignore
# excludes — keeping dataset terms from reaching this source tree. Only corpora
# whose licences permit commercial use are fetched; the restricted ones are
# skipped and can be inspected individually with `make info NAME=<dataset>`.

GO    ?= go
FETCH := ./scripts/fetch-dataset.sh

.PHONY: help download list info dry-run benchmark test build vet fmt clean-data

help:
	@echo "piiscrub targets:"
	@echo "  make download        fetch the commercially-usable datasets (~450 MB)"
	@echo "  make list            list known datasets and their licences"
	@echo "  make info NAME=...   licence and obligations for one dataset"
	@echo "  make dry-run         show what 'make download' would fetch, without fetching"
	@echo "  make benchmark       score the golden seed set and print the report"
	@echo "  make test            run the full test suite"
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

clean-data:
	rm -rf testdata/external
	@echo "removed testdata/external — run 'make download' to refetch"
