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

# ONNX backend build inputs. github.com/daulet/tokenizers links a static Rust
# library through cgo, and that library is NOT part of the Go module cache, so a
# plain `go build -tags onnx` fails to link until it is present. The pinned
# release archive is verified against this SHA-256 and unpacked into .cache/
# (gitignored). github.com/yalue/onnxruntime_go needs no library at build time:
# it compiles against vendored headers and loads libonnxruntime at runtime from
# the -onnxruntime-lib path.
#
# The asset and checksum are for linux/x86_64. Another platform must add its own
# pair from the same pinned release before the targets can work there.
ONNX_TOKENIZERS_VERSION := v1.27.0
ONNX_TOKENIZERS_ASSET   := libtokenizers.linux-x86_64.tar.gz
ONNX_TOKENIZERS_SHA256  := 72556cdca798dd4ea7cdaba308e5f0d68a8cb93b67c96edf485b7a0edd7b07f4
ONNX_TOKENIZERS_URL     := https://github.com/daulet/tokenizers/releases/download/$(ONNX_TOKENIZERS_VERSION)/$(ONNX_TOKENIZERS_ASSET)
ONNX_TOKENIZERS_DIR     := $(CURDIR)/.cache/tokenizers
ONNX_CGO_LDFLAGS        := -L$(ONNX_TOKENIZERS_DIR)

.PHONY: help download list info dry-run benchmark test test-race test-tools build vet fmt \
        corpus corpus-convert corpus-anonymize corpus-clean corpus-nlp clean-data presidio presidio-fetch \
        build-sidecar test-sidecar vet-sidecar docs-sync test-ner docs-check \
        fetch-libtokenizers build-sidecar-onnx test-sidecar-onnx onnx-check

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
	@echo "  make fetch-libtokenizers  download the pinned libtokenizers.a (needed by the onnx tag)"
	@echo "  make build-sidecar-onnx   build the sidecar with the onnx backend (-tags onnx)"
	@echo "  make test-sidecar-onnx    run the sidecar tests with -tags onnx"
	@echo "  make onnx-check      compile gate for the onnx tag (no download, no model needed)"
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

# NER_BACKEND selects the sidecar's backend. The default keeps every existing
# invocation unchanged. onnx needs a tagged build plus three artifacts:
#   NER_ONNX_MODEL        a DIRECTORY holding model.onnx and config.json (the
#                         sidecar's -model flag; the name is historical)
#   NER_ONNX_TOKENIZER    the tokenizer.json file
#   NER_ONNX_RUNTIME_LIB  the libonnxruntime shared library
NER_BACKEND ?= prose
NER_ONNX_MODEL ?=
NER_ONNX_TOKENIZER ?=
NER_ONNX_RUNTIME_LIB ?=
CORPUS_NLP_TAGS := $(if $(filter onnx,$(NER_BACKEND)),-tags onnx,)
CORPUS_NLP_CGO  := $(if $(filter onnx,$(NER_BACKEND)),CGO_LDFLAGS="$(ONNX_CGO_LDFLAGS)",)

corpus-nlp: build-sidecar $(if $(filter onnx,$(NER_BACKEND)),fetch-libtokenizers,)
	@if curl -sf $(NER_URL)/healthz >/dev/null 2>&1; then \
		echo "a sidecar is already listening on $(NER_URL); stop it first." >&2; \
		exit 1; \
	fi
	@cd sidecar && $(CORPUS_NLP_CGO) $(GO) build $(CORPUS_NLP_TAGS) -o "$(CURDIR)/$(NER_BIN)" ./cmd/piiscrub-sidecar
	@"$(CURDIR)/$(NER_BIN)" -addr $(NER_ADDR) -backend $(NER_BACKEND) \
		$(if $(NER_ONNX_MODEL),-model "$(NER_ONNX_MODEL)") \
		$(if $(NER_ONNX_TOKENIZER),-tokenizer "$(NER_ONNX_TOKENIZER)") \
		$(if $(NER_ONNX_RUNTIME_LIB),-onnxruntime-lib "$(NER_ONNX_RUNTIME_LIB)") \
		& echo $$! > .tmp-piiscrub-sidecar.pid
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

# Downloads the pinned libtokenizers.a into .cache/tokenizers. Run it once on a
# new machine. The archive is kept beside the library and its SHA-256 is checked
# on every run, so a corrupt or substituted archive is caught even when the
# library is already extracted. The library is extracted into a temporary
# directory and renamed into place, so an interrupted run never leaves a partial
# or stale libtokenizers.a. A different archive would be linked into the binary,
# and the failure it caused later would look like a model bug.
#
# The pinned asset is linux/x86_64 only, so any other host fails here with a
# message instead of a checksum error on a wrong-platform download.
fetch-libtokenizers:
	@test "$$(uname -sm)" = "Linux x86_64" || { \
		echo "fetch-libtokenizers: the pinned $(ONNX_TOKENIZERS_ASSET) is Linux x86_64 only; this host is $$(uname -sm)." >&2; \
		exit 1; \
	}
	@set -e; \
	mkdir -p "$(ONNX_TOKENIZERS_DIR)"; \
	archive="$(ONNX_TOKENIZERS_DIR)/$(ONNX_TOKENIZERS_ASSET)"; \
	lib="$(ONNX_TOKENIZERS_DIR)/libtokenizers.a"; \
	if [ ! -f "$$archive" ]; then \
		curl -sSLf -o "$$archive.part" "$(ONNX_TOKENIZERS_URL)"; \
		mv "$$archive.part" "$$archive"; \
	fi; \
	echo "$(ONNX_TOKENIZERS_SHA256)  $$archive" | sha256sum -c - || { \
		echo "fetch-libtokenizers: $$archive does not match the pinned SHA-256; delete it and rerun." >&2; \
		exit 1; \
	}; \
	if [ ! -f "$$lib" ] || [ "$$archive" -nt "$$lib" ]; then \
		tmp=$$(mktemp -d "$(ONNX_TOKENIZERS_DIR)/extract.XXXXXX"); \
		trap 'rm -rf "$$tmp"' EXIT; \
		tar -xzf "$$archive" -C "$$tmp"; \
		mv "$$tmp/libtokenizers.a" "$$lib.part"; \
		mv "$$lib.part" "$$lib"; \
	fi; \
	echo "libtokenizers.a ready in $(ONNX_TOKENIZERS_DIR)"

# The CI compile gate for the onnx tag. It does NOT download anything: a missing
# libtokenizers.a is a precondition failure that names the command which fixes it.
# No model or ONNX Runtime installation is needed to compile.
onnx-check:
	@test -f "$(ONNX_TOKENIZERS_DIR)/libtokenizers.a" || { \
		echo "onnx-check: $(ONNX_TOKENIZERS_DIR)/libtokenizers.a is missing." >&2; \
		echo "onnx-check: run 'make fetch-libtokenizers' first (pinned $(ONNX_TOKENIZERS_VERSION))." >&2; \
		exit 1; \
	}
	cd sidecar && CGO_LDFLAGS="$(ONNX_CGO_LDFLAGS)" $(GO) build -tags onnx ./...

build-sidecar-onnx: fetch-libtokenizers
	cd sidecar && CGO_LDFLAGS="$(ONNX_CGO_LDFLAGS)" $(GO) build -tags onnx ./...

test-sidecar-onnx: fetch-libtokenizers
	cd sidecar && CGO_LDFLAGS="$(ONNX_CGO_LDFLAGS)" $(GO) test -tags onnx ./...

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
