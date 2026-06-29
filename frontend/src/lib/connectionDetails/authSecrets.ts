import { importIdentity, importPassword } from '../../stores/api';

const KEY_FILE_ACCEPT = '.pem,.key,.id_rsa,.id_ecdsa,.id_ed25519,*';
export const MASKED_PASSWORD = '********';

export async function pickAndImportIdentity(): Promise<string | null> {
  return new Promise((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = KEY_FILE_ACCEPT;
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) {
        resolve(null);
        return;
      }
      const text = await file.text();
      const base64 = btoa(text);
      const kid = await importIdentity(base64, file.name);
      resolve(kid || null);
    };
    input.click();
  });
}

export async function importPasswordIfChanged(
  value: string,
  label: string,
): Promise<string | null> {
  if (!value || value === MASKED_PASSWORD) return null;
  const pwId = await importPassword(value, label);
  return pwId || null;
}
