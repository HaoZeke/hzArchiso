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
repoDir=/repo/hzarchiso


.PHONY: check
check:
	./scripts/hzarchiso-profile-check $(buildDir)
	./scripts/hzarchiso-usability-selftest $(buildDir)
	./scripts/hzarchiso-profile-check-selftest
	./scripts/hzarchiso-aur-repo-check-selftest
	./scripts/hzarchiso-aur-repo-build-selftest

.PHONY: aur-repo
aur-repo:
	./scripts/hzarchiso-aur-repo-build $(repoDir)

.PHONY: repo-check
repo-check:
	./scripts/hzarchiso-aur-repo-check $(repoDir) aurpkgs.txt

.PHONY: tui
tui:
	cd $(installerDir) && go build -buildvcs=false -trimpath -ldflags="-s -w" -o ../../$(tuiBin) .

.PHONY: tui-test
tui-test:
	cd $(installerDir) && go test ./...

.PHONY: tui-check
tui-check: tui-test tui
	./scripts/hz-install-tui-selftest

.PHONY: build
build: tui check repo-check
	sudo mkarchiso -v -w $(workDir) -o $(outDir) $(buildDir)

.PHONY: install
install: build

.PHONY: clean
clean:
	sudo rm -rf -- $(workDir) $(outDir)

# end
