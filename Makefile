REGION ?= ee
OUT    ?= ./drive

.PHONY: test sample drive run clean

## Run every test (Go launcher + Python builder).
test:
	cd launcher && go vet ./... && go test ./...
	python3 -m unittest discover -s tests

## Small trial drive (one town, no library) in ./drive.
sample:
	python3 -m builder --region ee-sample --out $(OUT)

## Full drive: make drive REGION=ee OUT=/Volumes/VARU
drive:
	python3 -m builder --region $(REGION) --out $(OUT)

## Start the launcher from source against the drive in OUT.
run:
	cd launcher && go run . -root "$(abspath $(OUT))"

clean:
	rm -rf ./drive
