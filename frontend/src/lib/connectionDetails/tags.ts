export const MAX_TAG_LENGTH = 30;

export function isTagTooLong(value: string): boolean {
  return value.trim().length > MAX_TAG_LENGTH;
}

export function isValidNewTag(value: string): boolean {
  const trimmed = value.trim();
  return trimmed.length > 0 && trimmed.length <= MAX_TAG_LENGTH;
}

export function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  return `hsl(${Math.abs(hash) % 360}, 50%, 40%)`;
}
