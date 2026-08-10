// Update-check RPC wrapper. The backend does the version comparison and owns the setting that
// permits the request; this only reads the last result. No store access here.
import { callBackend } from '../backend/callBackend';

export interface UpdateStatus {
  currentVersion: string;
  latestVersion: string;
  releaseUrl: string;
  updateAvailable: boolean;
  // checked distinguishes "we looked and you are current" from "we never looked" — the second
  // happens while the vault is locked or when the user turned the check off, and the UI must not
  // present it as a clean bill of health.
  checked: boolean;
}

export const NO_UPDATE_STATUS: UpdateStatus = {
  currentVersion: '',
  latestVersion: '',
  releaseUrl: '',
  updateAvailable: false,
  checked: false,
};

// fetchUpdateStatus returns the last known result without triggering a request. The check itself
// runs on the backend when the vault opens; a component that mounts later reads it from here.
export async function fetchUpdateStatus(): Promise<UpdateStatus> {
  return callBackend(
    'Get update status',
    NO_UPDATE_STATUS,
    async (app) => {
      if (!app.GetUpdateStatus) return NO_UPDATE_STATUS;
      // A build whose backend has no update service answers with nothing at all. The caller
      // renders a banner from this, so an absent answer has to become "we never looked" rather
      // than an undefined that reads as false everywhere it is touched.
      return ((await app.GetUpdateStatus()) as UpdateStatus | undefined) ?? NO_UPDATE_STATUS;
    },
  );
}
