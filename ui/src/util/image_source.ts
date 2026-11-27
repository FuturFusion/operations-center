// The local image source holds the uploaded images. It is read only.
export const localImageSourceName = "local";

// "create" clashes with the route to create a source.
const reservedImageSourceNames = [localImageSourceName, "create"];

export const validateImageSourceName = (value: string): string | undefined => {
  if (!/^[a-z0-9_]+$/.test(value)) {
    return "Name may only contain lower case letters, digits and underscores";
  }

  if (reservedImageSourceNames.includes(value)) {
    return `Name "${value}" is reserved`;
  }

  return undefined;
};
