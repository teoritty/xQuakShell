import { importPassword } from '../../api/credentials';

const KEY_FILE_ACCEPT = '.pem,.key,.id_rsa,.id_ecdsa,.id_ed25519,*';
export const MASKED_PASSWORD = '********';

export async function importPasswordIfChanged(
  value: string,
  label: string,
): Promise<string | null> {
  if (!value || value === MASKED_PASSWORD) return null;
  const pwId = await importPassword(value, label);
  return pwId || null;
}
