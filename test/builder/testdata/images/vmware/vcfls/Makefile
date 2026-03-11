ALL: build

IMAGE_NAME := Vcf-License-Server-9.1.0.0.25262073

IMAGE_DISKS := $(IMAGE_NAME)-disk1.vmdk \
               $(IMAGE_NAME)-disk2.vmdk

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