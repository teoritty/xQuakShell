// The team half of the Wails bridge (ADR-023): team vaults, members and credential bindings.

export interface Team {
  vault: string;
  name: string;
  role: 'owner' | 'admin' | 'editor' | 'viewer';
}

export interface TeamMember {
  account: string;
  username: string;
  fingerprint: string;
  role: 'owner' | 'admin' | 'editor' | 'viewer';
  verified: boolean;
  current: boolean;
}

export interface TeamInvitee {
  username: string;
  fingerprint: string;
  known: boolean;
  verified: boolean;
}

export interface TeamInviteRequest {
  vault: string;
  username: string;
  fingerprint: string;
  role: string;
  verified: boolean;
}

export interface CredentialBinding {
  vault: string;
  connection: string;
  user: string;
  identity?: string;
  password?: string;
  suspendedBy?: string;
}

export interface TeamGateway {
  ListTeams(): Promise<Team[]>;
  CreateTeam(name: string): Promise<string>;
  LookupTeamInvitee(username: string): Promise<TeamInvitee>;
  InviteToTeam(req: TeamInviteRequest): Promise<void>;
  ListTeamMembers(vault: string): Promise<TeamMember[]>;
  ChangeTeamRole(vault: string, account: string, role: string): Promise<void>;
  RemoveTeamMember(vault: string, account: string): Promise<void>;
  LeaveTeam(vault: string): Promise<void>;
  VerifyTeamMember(vault: string, account: string, fingerprint: string): Promise<void>;
  SetTeamPolicy(vault: string, retentionDays: number, requireVerified: boolean): Promise<void>;
  ListCredentialBindings(): Promise<CredentialBinding[]>;
  BindCredential(binding: CredentialBinding): Promise<void>;
  ConfirmCredentialBinding(vault: string, connection: string, user: string): Promise<void>;
  MoveConnectionToVault(connection: string, vault: string, folder: string): Promise<string>;
}
