package framework

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"

	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/podlogs"
)

// https://github.com/kubernetes/kubernetes/blob/3d6026499b674020b4f8eec11f0b8a860a330d8a/test/e2e/storage/podlogs/podlogs.go
// https://github-vcf.devops.broadcom.net/vcf/vks-gce2e/blob/cd04354b144b707d113b21a740f83f156cabaea5/test/e2e/lib/e2e/lib.go#L42
func WatchPodLogsAndEvents(ctx context.Context, cs kubernetes.Interface, podArtifactFolder, ns string) {
	// Needed in case directory permission bits get messed up due to umask.
	oldUMask := syscall.Umask(0)

	Expect(os.MkdirAll(podArtifactFolder, 0755)).To(Succeed(), "failed to create pod artifact log folder: %s", podArtifactFolder)

	logOutput, err := os.Create(filepath.Join(podArtifactFolder, "podStatus.txt"))
	Expect(err).NotTo(HaveOccurred(), "failed to create file for pod status: %v", err)

	// Reset the umask.
	syscall.Umask(oldUMask)

	to := podlogs.LogOutput{
		StatusWriter: logOutput,
		// LogPathPrefix is used as both the initial path and the start of the filename,
		//  so we prefix it with 'logs-' to ensure pod events correctly end up in the same
		//  directory, and it's possible to distinguish logs and events by looking at the prefix.
		LogPathPrefix: podArtifactFolder + "/logs-",
	}

	Expect(podlogs.CopyAllLogs(ctx, cs, ns, to)).To(Succeed(), "collect pod logs failed")

	podEventLogFile := filepath.Join(podArtifactFolder, "podEvents.txt")
	eventWriters := make([]io.Writer, 0)

	eventOutputFile, err := os.Create(podEventLogFile)
	if err != nil {
		e2eframework.Failf("failed to open event file %s : %+v", podEventLogFile, err)
	}

	eventWriters = append(eventWriters, eventOutputFile)

	eventOutput := io.MultiWriter(eventWriters...)

	var eventLogCloser io.Closer
	Expect(podlogs.WatchPods(ctx, cs, ns, eventOutput, eventLogCloser)).To(Succeed(), "error occurred during watching pod events")
}

func WatchPodLogsAndEventsInNamespaces(ctx context.Context, watchNsList []string, cs kubernetes.Interface, podArtifactFolder string) context.CancelFunc {
	watchesCtx, cancelWatches := context.WithCancel(ctx)

	for _, ns := range watchNsList {
		WatchPodLogsAndEvents(watchesCtx, cs, podArtifactFolder, ns)
	}

	return cancelWatches
}
