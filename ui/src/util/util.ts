export function bytesToHumanReadable(
  bytes: number,
  decimalPlaces: number = 2,
): string {
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"];
  if (bytes === 0) return "0 B";

  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const size = bytes / Math.pow(k, i);

  return `${size.toFixed(decimalPlaces)} ${units[i]}`;
}

export function humanReadableToBytes(input: string): number {
  const binaryUnits = [
    "B",
    "KiB",
    "MiB",
    "GiB",
    "TiB",
    "PiB",
    "EiB",
    "ZiB",
    "YiB",
  ];
  const decimalUnits = ["B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"];
  const regex =
    /^([\d.]+)\s*(B|KiB|MiB|GiB|TiB|PiB|EiB|ZiB|YiB|KB|MB|GB|TB|PB|EB|ZB|YB)$/i;

  const match = input.match(regex);
  if (!match) {
    throw new Error(
      "Invalid format. Example of valid formats: 1 KiB, 100 MiB, 1.5 GiB, 1.5 GB",
    );
  }

  const value = parseFloat(match[1]);
  const unit = match[2];

  // Check if the unit is in the binary (base-1024) or decimal (base-1000) system
  let index = binaryUnits.findIndex(
    (u) => u.toLowerCase() === unit.toLowerCase(),
  );
  let base = 1024; // Default to binary

  if (index === -1) {
    index = decimalUnits.findIndex(
      (u) => u.toLowerCase() === unit.toLowerCase(),
    );
    base = 1000;
  }

  if (index === -1) {
    throw new Error(
      `Invalid unit: ${unit}. Allowed units are: ${binaryUnits.join(", ")}, ${decimalUnits.join(", ")}`,
    );
  }

  return Math.round(value * Math.pow(base, index));
}
