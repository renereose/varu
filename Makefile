REGION  ?= ee
OUT     ?= ./drive
VERSION ?= dev

.PHONY: test sample drive run clean release-content desktop android

## Run every test (Go router, launcher and installer, desktop app, Python builder).
test:
	cd router && go vet ./... && go test ./... && GOOS=linux GOARCH=arm GOARM=6 go build ./...
	cd launcher && go vet ./... && go test ./...
	cd installer && go vet ./... && go test ./...
	node --check desktop/main.js && node --check desktop/preload.js
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

## Release files the installers download: make release-content REGION=ee
## The public Overpass server is often briefly overloaded (HTTP 502), so a
## failed build runs again after a pause; finished steps are skipped.
release-content:
	@for try in 1 2 3; do \
	  python3 -m builder --region $(REGION) --out build/$(REGION) --skip library && break; \
	  if [ "$$try" = 3 ]; then exit 1; fi; \
	  echo "Retrying the failed steps in 2 minutes"; sleep 120; \
	done
	python3 -m builder.release --region $(REGION) --drive build/$(REGION) --out dist

## The Varu app for macOS, Windows and Linux, into dist/ (needs macOS and Node)
desktop:
	sh desktop/build.sh $(VERSION) dist

## dist/Varu.apk (needs an Android SDK and a JDK)
android:
	sh android/build.sh $(VERSION) dist

clean:
	rm -rf ./drive ./build ./dist
