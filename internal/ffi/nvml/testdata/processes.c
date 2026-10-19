#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

// Layouts from NVIDIA/nvidia-settings tag 535.54.03, src/nvml.h.
// v2 includes both MIG IDs; only this release's v3 adds protected memory.
typedef struct {
    unsigned int pid;
    unsigned long long usedGpuMemory;
    unsigned int gpuInstanceId;
    unsigned int computeInstanceId;
} nvmlProcessInfo_v2_t;

typedef struct {
    unsigned int pid;
    unsigned long long usedGpuMemory;
    unsigned int gpuInstanceId;
    unsigned int computeInstanceId;
    unsigned long long usedGpuCcProtectedMemory;
} nvmlProcessInfo_early535_t;

_Static_assert(sizeof(nvmlProcessInfo_v2_t) == 24, "unexpected v2 ABI");
_Static_assert(sizeof(nvmlProcessInfo_early535_t) == 32, "unexpected early R535 ABI");

static unsigned int probe_count;
static unsigned int fill_count;

void fixtureSetCounts(unsigned int probe, unsigned int fill) {
    probe_count = probe;
    fill_count = fill;
}

void fixtureProcessLayout(uint64_t *layout) {
    layout[0] = sizeof(nvmlProcessInfo_v2_t);
    layout[1] = offsetof(nvmlProcessInfo_v2_t, pid);
    layout[2] = offsetof(nvmlProcessInfo_v2_t, usedGpuMemory);
    layout[3] = offsetof(nvmlProcessInfo_v2_t, gpuInstanceId);
    layout[4] = offsetof(nvmlProcessInfo_v2_t, computeInstanceId);
}

int nvmlInit_v2(void) { return 0; }
int nvmlShutdown(void) { return 0; }
int nvmlDeviceGetCount_v2(unsigned int *count) { *count = 1; return 0; }
int nvmlDeviceGetHandleByIndex_v2(unsigned int index, void **device) {
    (void)index;
    *device = (void *)(uintptr_t)1;
    return 0;
}
int nvmlDeviceGetNvLinkState(void *device, unsigned int link, unsigned int *active) {
    (void)device; (void)link; (void)active;
    return 3;
}
// Mandatory exports not invoked by the process-enumeration tests.
int nvmlDeviceGetPcieThroughput(void *device, unsigned int counter, unsigned int *value) {
    (void)device; (void)counter; (void)value;
    return 3;
}
int nvmlDeviceGetFieldValues(void *device, int count, void *values) {
    (void)device; (void)count; (void)values;
    return 3;
}
int nvmlDeviceGetUUID(void *device, char *value, unsigned int length) {
    (void)device; (void)value; (void)length;
    return 3;
}
int nvmlDeviceGetName(void *device, char *value, unsigned int length) {
    (void)device; (void)value; (void)length;
    return 3;
}

#if HAVE_V2
int nvmlDeviceGetComputeRunningProcesses_v2(void *device, unsigned int *count,
                                           nvmlProcessInfo_v2_t *infos) {
    (void)device;
    if (infos == NULL) {
        *count = probe_count;
        return probe_count == 0 ? 0 : 7;
    }
    unsigned int capacity = *count;
    *count = fill_count;
    if (capacity < fill_count) return 7;
    for (unsigned int i = 0; i < fill_count; i++) {
        infos[i].pid = 1000 + i;
        infos[i].usedGpuMemory = (1ULL << 40) + i;
        infos[i].gpuInstanceId = 200 + i;
        infos[i].computeInstanceId = 300 + i;
    }
    return 0;
}
#endif

#if HAVE_V3
int nvmlDeviceGetComputeRunningProcesses_v3(void *device, unsigned int *count,
                                           nvmlProcessInfo_early535_t *infos) {
    (void)device; (void)count; (void)infos;
    // Fail at the unsafe ABI selection instead of corrupting the test process.
    fputs("selected incompatible early-R535 v3 process ABI\n", stderr);
    exit(1);
}
#endif
