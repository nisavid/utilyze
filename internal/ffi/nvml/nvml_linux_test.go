//go:build linux

package nvml

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

func TestComputeProcessesNativeABI(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("native NVML ABI test requires a C compiler")
	}
	for _, symbols := range []struct {
		name string
		v2   string
		v3   string
	}{
		{"both", "1", "1"},
		{"v2_only", "1", "0"},
		{"v3_only", "0", "1"},
		{"neither", "0", "0"},
	} {
		t.Run(symbols.name, func(t *testing.T) {
			lib := filepath.Join(t.TempDir(), "libnvml_fixture.so")
			cmd := exec.Command(cc, "-std=c11", "-Wall", "-Wextra", "-Werror", "-shared", "-fPIC",
				"-DHAVE_V2="+symbols.v2, "-DHAVE_V3="+symbols.v3,
				"-o", lib, "testdata/processes.c")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("compile NVML fixture: %v\n%s", err, output)
			}
			// Isolate the package's global native function pointers and library lifetime.
			cmd = exec.Command(os.Args[0], "-test.run=^TestComputeProcessesNativeHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "UTLZ_TEST_NVML_LIBRARY="+lib, "UTLZ_TEST_NVML_V2="+symbols.v2)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("NVML ABI subprocess: %v\n%s", err, output)
			}
		})
	}
}

func TestComputeProcessesNativeHelper(t *testing.T) {
	path := os.Getenv("UTLZ_TEST_NVML_LIBRARY")
	if path == "" {
		t.Skip("run through TestComputeProcessesNativeABI")
	}
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(lib)
	loadSymbols(lib)

	var nativeLayout func(layout []uint64)
	var setCounts func(probe, fill uint32)
	purego.RegisterLibFunc(&nativeLayout, lib, "fixtureProcessLayout")
	purego.RegisterLibFunc(&setCounts, lib, "fixtureSetCounts")
	var record nvmlProcessInfo
	goLayout := []uint64{
		uint64(unsafe.Sizeof(record)),
		uint64(unsafe.Offsetof(record.Pid)),
		uint64(unsafe.Offsetof(record.UsedGpuMemory)),
		uint64(unsafe.Offsetof(record.GpuInstanceId)),
		uint64(unsafe.Offsetof(record.ComputeInstanceId)),
	}
	layout := make([]uint64, len(goLayout))
	nativeLayout(layout)
	for i, want := range layout {
		if goLayout[i] != want {
			t.Fatalf("Go ABI layout[%d] = %d, native v2 = %d", i, goLayout[i], want)
		}
	}

	client := &Client{devices: make(map[int]*deviceState)}
	if os.Getenv("UTLZ_TEST_NVML_V2") == "0" {
		got, err := client.GetComputeProcesses(0)
		if got != nil || err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("missing v2: got %v, %v; want unavailable error", got, err)
		}
		return
	}
	for _, tc := range []struct {
		name        string
		probe, fill uint32
	}{
		{"empty", 0, 0},
		{"single", 1, 1},
		{"thirteen", 13, 13},
		{"many", 64, 64},
		{"growth_within_slack", 13, 17},
		{"growth_beyond_slack", 13, 18},
		{"shrink", 13, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setCounts(tc.probe, tc.fill)
			got, err := client.GetComputeProcesses(0)
			if tc.fill > tc.probe+4 {
				if got != nil || err == nil || !strings.Contains(err.Error(), "fill: 7") {
					t.Fatalf("count growth: got %v, %v; want insufficient-size error", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != int(tc.fill) {
				t.Fatalf("got %d records, want %d", len(got), tc.fill)
			}
			for i, record := range got {
				want := ProcessInfo{
					PID:                1000 + i,
					UsedGpuMemoryBytes: (uint64(1) << 40) + uint64(i),
					GpuInstanceID:      200 + uint32(i),
					ComputeInstanceID:  300 + uint32(i),
				}
				if record != want {
					t.Fatalf("record %d = %+v, want %+v", i, record, want)
				}
			}
		})
	}
}
