ALL: build

IMAGE_NAME := O11N_VA-9.1.0.0.25262048

IMAGE_DISKS := $(IMAGE_NAME)-system.vmdk \
               $(IMAGE_NAME)-logs.vmdk \
               $(IMAGE_NAME)-home.vmdk \
               $(IMAGE_NAME)-data.vmdk	

OVA := $(IMAGE_NAME).ova
YAML := $(IMAGE_NAME).yaml

$(IMAGE_NAME).mf: $(IMAGE_NAME).ovf $(IMAGE_DISKS)
	sha1sum $(^) >$(IMAGE_NAME).mf
	@cat $(@)

$(IMAGE_NAME).ova: $(IMAGE_NAME).mf
	@rm -f $(@)
	tar cvf $(@) $(IMAGE_NAME).ovf $(<) $(IMAGE_DISKS) 
	tar tf $(@)

$(IMAGE_NAME).yaml: $(IMAGE_NAME).ovf
	go run ../../ovf2yaml.go $(abspath $(?)) >$(@)

build: $(OVA) $(YAML)

clean:
	rm -f $(OVA) $(YAML)