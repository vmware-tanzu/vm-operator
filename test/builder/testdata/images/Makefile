ALL: build

IMAGE_1_NAME := ttylinux-pc_i486-16.1
IMAGE_1_DISK := $(IMAGE_1_NAME)-disk1.vmdk
IMAGE_1_NVRAM := $(IMAGE_1_NAME)-2.nvram

OVAS := $(IMAGE_1_NAME).ova

$(IMAGE_1_NAME).mf: $(IMAGE_1_NAME).ovf $(IMAGE_1_DISK) $(IMAGE_1_NVRAM)
	sha1sum $(^) >$(IMAGE_1_NAME).mf
	@cat $(@)

$(IMAGE_1_NAME).ova: $(IMAGE_1_NAME).mf
	@rm -f $(@)
	tar cvf $(@) $(IMAGE_1_NAME).ovf $(<) $(IMAGE_1_DISK) $(IMAGE_1_NVRAM)
	tar tf $(@)

build: $(OVAS)

clean:
	rm -f $(OVAS)