REGION  ?= ee
OUT     ?= ./drive
VERSION ?= dev

.PHONY: test sample drive run clean release-content installer-windows installer-macos android

## Run every test (Go launcher and installer, Python builder).
test:
	cd launcher && go vet ./... && go test ./...
	cd installer && go vet ./... && go test ./...
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
release-content:
	python3 -m builder --region $(REGION) --out build/$(REGION) --skip library
	python3 -m builder.release --region $(REGION) --drive build/$(REGION) --out dist

## dist/Varu-Installer.exe
installer-windows:
	cd installer && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
	  -ldflags "-s -w -H windowsgui -X main.version=$(VERSION)" -o ../dist/Varu-Installer.exe .

## dist/Varu-Installer-macOS.zip (needs macOS)
installer-macos:
	sh installer/macos/build-app.sh $(VERSION) dist

## dist/Varu.apk (needs an Android SDK and a JDK)
android:
	sh android/build.sh $(VERSION) dist

clean:
	rm -rf ./drive ./build ./dist
