##
# Builder
#
# @file
# @version 0.1

# Makefile Variables

buildDir=hzLinux

# Build Variables

application="HaoZeke's ArchLinux"
isoName='hzlinux_V'
# name_ver-ddmmyy
label='HZLIN_V-211222'
publisher='Rohit Goswami (HaoZeke) <rohit.goswami[at]aol.com>'


.PHONY: install
install:
	cd $(buildDir) && sudo ./build.sh -v \
  -A $(application) \
  -N $(isoName) \
  -L $(label) \
  -P $(publisher)

.PHONY: clean
clean:
	cd $(buildDir) && \
	sudo rm -v work/build.make_* && \
	sudo find work/etc/ -type l -delete

# end
