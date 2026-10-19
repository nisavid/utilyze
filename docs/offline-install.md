# Offline / airgapped installation

These instructions are for machines without internet access. The installer requires internet at install time, and unprivileged `utlz` runs can offer to download missing CUPTI libraries from PyPI — both need to be handled manually. Privileged runs never download CUPTI or load it from a user cache.

**You may not need the CUPTI files.** If the airgapped machine already has the CUDA toolkit or the `nvidia-cuda-cupti-cu12` pip package, `utlz` may find CUPTI on its own. For privileged runs, this must be a system installation: both libraries and every directory in their resolved paths must be root-owned and not group- or world-writable. User packages and caches are only supported for unprivileged runs.

<details>
<summary>Check whether CUPTI is already present</summary>

Run this on the airgapped machine to find potential installations. A result in the loader cache alone does not establish that a path is permitted for privileged loading:

```bash
ldconfig -p | grep libnvperf_host
ls /usr/local/cuda*/targets/*/lib/libnvperf_host.so \
   /usr/lib/python*/*-packages/nvidia/cuda_cupti/lib/libnvperf_host.so \
   /usr/local/lib/python*/*-packages/nvidia/cuda_cupti/lib/libnvperf_host.so \
   ~/.local/lib/python*/site-packages/nvidia/cuda_cupti/lib/libnvperf_host.so 2>/dev/null
```

</details>

## 1. On an internet-connected machine, gather two files

**a. The `utlz` binary** — from the [latest release](https://github.com/systalyze/utilyze/releases/latest):

- DGX / x86_64: `utlz-linux-amd64`
- Grace / aarch64: `utlz-linux-arm64`

**b. CUPTI 12+** — download the latest `nvidia-cuda-cupti-cu12` wheel from PyPI for your airgapped machine's architecture:

```bash
# DGX / x86_64
pip3 download nvidia-cuda-cupti-cu12 --no-deps --only-binary=:all: \
  --platform manylinux_2_25_x86_64 --python-version 3 -d .

# Grace / aarch64
pip3 download nvidia-cuda-cupti-cu12 --no-deps --only-binary=:all: \
  --platform manylinux_2_25_aarch64 --python-version 3 -d .
```

If `pip` isn't available, you can browse [the file list on PyPI](https://pypi.org/project/nvidia-cuda-cupti-cu12/#files) and download the matching wheel manually.

Extract the two `.so` files you need (replace `<version>` with the version in the wheel filename, e.g. `12.9.79`):

```bash
unzip -j nvidia_cuda_cupti_cu12-<version>-*.whl \
  'nvidia/cuda_cupti/lib/libnvperf_host.so' \
  'nvidia/cuda_cupti/lib/libnvperf_target.so' \
  -d ./cupti
```

Copy `utlz-linux-{amd64,arm64}` and the `cupti/` directory to the airgapped machine.

## 2. On the airgapped machine, install

Pick one of the two options below. The commands use `utlz-linux-amd64`; on Grace / aarch64, substitute `utlz-linux-arm64`.

### Option 1: Run with `sudo` every time (recommended if you have sudo)

```bash
sudo install -m 0755 utlz-linux-amd64 /usr/local/bin/utlz
sudo ln -sf utlz /usr/local/bin/utilyze
sudo install -d -o root -g root -m 0755 /usr/local/lib/utilyze/cupti
sudo install -o root -g root -m 0644 cupti/libnvperf_host.so cupti/libnvperf_target.so /usr/local/lib/utilyze/cupti/
```

Run:

```bash
sudo utlz
```

All parent directories, including `/usr/local/lib/utilyze`, must also be root-owned and not group- or world-writable. Start `utlz` with `LD_LIBRARY_PATH` unset, not merely empty; privileged library loading rejects it rather than letting it redirect dependencies. Administrator-installed libraries and the system dynamic linker configuration remain trusted.

### Option 2: Run without `sudo` (requires one-time host admin setup)

NVIDIA restricts GPU profiling counters to root by default. A host admin can disable this restriction once, after which any user on the box can run `utlz` without `sudo`.

Host admin step (one time only, requires sudo):

```bash
echo 'options nvidia NVreg_RestrictProfilingToAdminUsers=0' | sudo tee /etc/modprobe.d/nvidia-profiling.conf
# either reboot, or reload the driver (stops all GPU processes):
sudo modprobe -rf nvidia_uvm nvidia_drm nvidia_modeset nvidia && sudo modprobe nvidia
```

End user install (no sudo):

```bash
mkdir -p ~/.local/bin
install -m 0755 utlz-linux-amd64 ~/.local/bin/utlz
ln -sf utlz ~/.local/bin/utilyze
# make sure ~/.local/bin is on your PATH

cache_root="${XDG_CACHE_HOME:-$HOME/.cache}"
mkdir -p "$cache_root/utlz/cupti-<version>"
cp cupti/libnvperf_host.so cupti/libnvperf_target.so "$cache_root/utlz/cupti-<version>/"
```

Run:

```bash
utlz
```

## 3. Verify

```bash
utlz --version
```

Then start it (`sudo utlz` or `utlz` depending on path). You should see the live monitoring TUI. If `utlz` cannot find CUPTI, check that both libraries are directly inside `/usr/local/lib/utilyze/cupti/` for privileged runs, or `$cache_root/utlz/cupti-<version>/` for unprivileged runs, where `cache_root="${XDG_CACHE_HOME:-$HOME/.cache}"` as in the install commands above. For privileged runs, also check ownership and permissions of the libraries and every parent directory. The root user's cache is not a privileged search location.
