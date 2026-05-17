##
# Builder
#
# @file
# @version 0.1

# Makefile Variables

buildDir=hzLinux
workDir=work
outDir=out


.PHONY: check
check:
	./scripts/hzarchiso-profile-check $(buildDir)

.PHONY: build
build: check
	sudo mkarchiso -v -w $(workDir) -o $(outDir) $(buildDir)

.PHONY: install
install: build

.PHONY: clean
clean:
	sudo rm -rf -- $(workDir) $(outDir)

# end
