ALL: build

IMAGE_NAME := Operations-Cloud-Proxy-9.1.0.0.25262086

IMAGE_DISKS := $(IMAGE_NAME)-system.vmdk \
               $(IMAGE_NAME)-data.vmdk \
               $(IMAGE_NAME)-cloud-components.vmdk \
               $(IMAGE_NAME)-unified.vmdk

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