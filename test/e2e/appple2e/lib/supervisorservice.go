package lib

type SupervisorServiceType int

const (
	Custom SupervisorServiceType = iota
	VSphereApp
)

func (sst SupervisorServiceType) String() string {
	return [...]string{"custom-service", "vsphere-app-service", "carvel-service"}[sst]
}

const (
	VapiNotFoundErrMsg              = "Server error: com.vmware.vapi.std.errors.NotFound"
	VapiUnauthorizedErrMsg          = "Server error: com.vmware.vapi.std.errors.Unauthorized"
	VapiAlreadyInDesiredStateErrMsg = "Server error: com.vmware.vapi.std.errors.AlreadyInDesiredState"
)
