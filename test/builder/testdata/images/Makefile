ALL: build

DISK_1 := system.vmdk
DISK_2 := data.vmdk
DISK_3 := extra_data.vmdk

IMAGE_1_NAME := ttylinux-pc_i486-16.1
IMAGE_1_DISKS := $(DISK_1)
IMAGE_1_NVRAM := $(IMAGE_1_NAME).nvram

IMAGE_2_NAME := uber
IMAGE_2_DISKS := $(DISK_1) $(DISK_2) $(DISK_3)
IMAGE_2_NVRAM := $(IMAGE_2_NAME).nvram

OVAS := $(IMAGE_1_NAME).ova $(IMAGE_2_NAME).ova
YAML := $(IMAGE_1_NAME).yaml $(IMAGE_2_NAME).yaml

$(IMAGE_1_NAME).mf: $(IMAGE_1_NAME).ovf $(IMAGE_1_NVRAM) $(IMAGE_1_DISKS)
	sha1sum $(^) >$(IMAGE_1_NAME).mf
	@cat $(@)

$(IMAGE_1_NAME).ova: $(IMAGE_1_NAME).mf
	@rm -f $(@)
	tar cvf $(@) $(IMAGE_1_NAME).ovf $(<) $(IMAGE_1_NVRAM) $(IMAGE_1_DISKS) 
	tar tf $(@)

$(IMAGE_1_NAME).yaml: $(IMAGE_1_NAME).ovf
	go run ./ovf2yaml.go $(?) >$(@)

$(IMAGE_2_NAME).mf: $(IMAGE_2_NAME).ovf $(IMAGE_2_NVRAM) $(IMAGE_2_DISKS)
	sha1sum $(^) >$(IMAGE_2_NAME).mf
	@cat $(@)

$(IMAGE_2_NAME).ova: $(IMAGE_2_NAME).mf
	@rm -f $(@)
	tar cvf $(@) $(IMAGE_2_NAME).ovf $(<) $(IMAGE_2_NVRAM) $(IMAGE_2_DISKS)
	tar tf $(@)

$(IMAGE_2_NAME).yaml: $(IMAGE_2_NAME).ovf
	go run ./ovf2yaml.go $(?) >$(@)

build: $(OVAS) $(YAML)

clean:
	rm -f $(OVAS)