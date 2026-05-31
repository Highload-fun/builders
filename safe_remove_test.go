package builders

import (
	"strings"
	"testing"
)

// /proc/self/mountinfo lines. Space-separated layout:
//   1=mountID 2=parentID 3=major:minor 4=root 5=mountPoint 6=options ...
// mountUnder reads field 5 (index 4) as the mount point.
const sampleNoMounts = `21 26 0:20 / /sys rw,nosuid,nodev,noexec,relatime shared:7 - sysfs sysfs rw
24 26 0:5 / /dev rw,nosuid shared:2 - devtmpfs udev rw,size=8123456k
30 24 0:21 / /tmp rw,nosuid,nodev shared:5 - tmpfs tmpfs rw
`

const sampleBindUnderDir = sampleNoMounts +
	`200 30 8:3 /usr/lib /tmp/build123456/usr/lib rw,relatime shared:5 - ext4 /dev/md2 rw
201 30 8:3 /lib /tmp/build123456/lib rw,relatime shared:5 - ext4 /dev/md2 rw
`

// A mount under /tmp/build10 must NOT be reported for dir /tmp/build1.
const sampleBoundary = sampleNoMounts +
	`300 30 8:3 /usr/lib /tmp/build10/usr/lib rw,relatime shared:5 - ext4 /dev/md2 rw
`

func TestMountUnder_DetectsBindUnderDir(t *testing.T) {
	mp, found := mountUnder(strings.NewReader(sampleBindUnderDir), "/tmp/build123456")
	if !found {
		t.Fatalf("mountUnder did not detect a bind mount under /tmp/build123456")
	}
	if mp != "/tmp/build123456/usr/lib" {
		t.Fatalf("mountUnder returned %q, want /tmp/build123456/usr/lib", mp)
	}
}

func TestMountUnder_NoMounts(t *testing.T) {
	if mp, found := mountUnder(strings.NewReader(sampleNoMounts), "/tmp/build123456"); found {
		t.Fatalf("mountUnder reported %q when no mount should match", mp)
	}
}

func TestMountUnder_ComponentBoundary(t *testing.T) {
	if mp, found := mountUnder(strings.NewReader(sampleBoundary), "/tmp/build1"); found {
		t.Fatalf("mountUnder spuriously matched %q for dir /tmp/build1 (prefix bug)", mp)
	}
}

func TestMountUnder_ExactDirMatch(t *testing.T) {
	mi := sampleNoMounts + "400 30 8:3 / /tmp/build123456 rw,relatime shared:5 - ext4 /dev/md2 rw\n"
	mp, found := mountUnder(strings.NewReader(mi), "/tmp/build123456")
	if !found || mp != "/tmp/build123456" {
		t.Fatalf("mountUnder failed exact-dir match: got %q found=%v", mp, found)
	}
}
