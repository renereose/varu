REGION  ?= ee
OUT     ?= ./drive
VERSION ?= dev
DRIVE   ?= build/$(REGION)

.PHONY: test sample drive run clean release-content desktop android pocket mockups

## Run every test (Go router, library reader, map files, launcher, installer, pocket renderer, desktop app, Python builder).
test:
	cd pmtiles && go vet ./... && go test ./...
	cd router && go vet ./... && go test ./... && GOOS=linux GOARCH=arm GOARM=6 go build ./...
	cd zim && go vet ./... && go test ./... && GOOS=linux GOARCH=arm GOARM=6 go build ./...
	cd launcher && go vet ./... && go test ./...
	cd installer && go vet ./... && go test ./...
	cd pocket && go vet ./... && go test ./... && GOOS=linux GOARCH=arm64 go build -o /dev/null . && GOOS=linux GOARCH=arm GOARM=6 go build -o /dev/null .
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

## The pocket device's renderer for a Raspberry Pi, into build/pocket/ (64-bit Pi OS, and 32-bit for a Pi Zero W)
pocket:
	cd pocket && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o ../build/pocket/varu-pocket-arm64 .
	cd pocket && GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 go build -trimpath -o ../build/pocket/varu-pocket-armv6 .

## E-ink mock-ups of the pocket device from a drive, into build/mockups/: make mockups DRIVE=/Volumes/VARU
mockups:
	mkdir -p build/mockups
	cd pocket && go build -o ../build/mockups/varu-pocket .
	@for place in tallinn:59.4370,24.7450 tartu:58.3801,26.7225 rouge:57.7290,26.9130; do \
	  for z in 10 12 14; do \
	    build/mockups/varu-pocket -drive "$(DRIVE)" -center "$${place#*:}" -zoom $$z -o "build/mockups/$${place%%:*}-$$z.png" && \
	    build/mockups/varu-pocket -drive "$(DRIVE)" -center "$${place#*:}" -zoom $$z -grey -o "build/mockups/$${place%%:*}-$$z-grey.png" || exit 1; \
	  done; \
	done
	build/mockups/varu-pocket -drive "$(DRIVE)" -center 59.4370,24.7450 -zoom 14 -size 648x480 -o build/mockups/tallinn-14-648x480.png
	build/mockups/varu-pocket -drive "$(DRIVE)" -center 59.4370,24.7450 -zoom 12 -size 648x480 -grey -o build/mockups/tallinn-12-648x480-grey.png
	build/mockups/varu-pocket -drive "$(DRIVE)" -legend -o build/mockups/legend.png
	build/mockups/varu-pocket -drive "$(DRIVE)" -legend -grey -o build/mockups/legend-grey.png

clean:
	rm -rf ./drive ./build ./dist
