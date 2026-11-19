#!/bin/bash

# Push the shared libraries of a locally built operations-centerd, which IncusOS
# does not ship, into an instance and point the operations-center unit at them
# until the next reboot.
#
# Usage: sideload-libs.sh [binary] [instance] [library directory]
set -euo pipefail

binary=${1:-bin/operations-centerd}
instance=${2:-OperationsCenter}
libdir=${3:-/root/dev/lib}

missing=$(ldd "${binary}" | grep "not found" || true)
if [ -n "${missing}" ]; then
  echo "Missing libraries of ${binary}, set LD_LIBRARY_PATH as printed by \"make deps\":" >&2
  echo "${missing}" >&2
  exit 1
fi

# The C library and the dynamic loader are part of IncusOS.
libs=$(ldd "${binary}" | awk '$2 == "=>" && $3 ~ /^\// {print $3}' | grep -vE '/(libc|libm|libpthread|libdl|librt|libresolv)\.so' || true)
if [ -z "${libs}" ]; then
  exit 0
fi

incus exec "${instance}" -- mkdir -p "${libdir}"
for lib in ${libs}; do
  incus file push "$(readlink -f "${lib}")" "${instance}${libdir}/$(basename "${lib}")"
done

# A drop-in below /run lasts until the next reboot, like the sideloaded binary.
incus exec "${instance}" -- sh -c "mkdir -p /run/systemd/system/operations-center.service.d && printf '[Service]\nEnvironment=LD_LIBRARY_PATH=%s\n' '${libdir}' > /run/systemd/system/operations-center.service.d/sideload-libs.conf && systemctl daemon-reload"
