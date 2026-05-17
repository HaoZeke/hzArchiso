##
# Builder
#
# @file
# @version 0.1

# Makefile Variables

buildDir=hzLinux
workDir=work
outDir=out
installerDir=installer/hz-install-tui
tuiBin=$(buildDir)/airootfs/usr/local/bin/hz-install-tui


.PHONY: check
check:
	./scripts/hzarchiso-profile-check $(buildDir)

.PHONY: tui
tui:
	cd $(installerDir) && go build -buildvcs=false -trimpath -ldflags="-s -w" -o ../../$(tuiBin) .

.PHONY: tui-check
tui-check: tui
	./scripts/hz-install-tui-selftest

.PHONY: build
build: tui check
	sudo mkarchiso -v -w $(workDir) -o $(outDir) $(buildDir)

.PHONY: install
install: build

.PHONY: clean
clean:
	sudo rm -rf -- $(workDir) $(outDir)

# end
