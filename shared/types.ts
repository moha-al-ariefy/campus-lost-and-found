export type Role = 'campus' | 'staff' | 'admin';
export interface Account { id: string; name: string; role: Role; holdingPoints: string[]; }
export interface PublicItem { id: string; category: string; location: string; dateFound: string; holdingPoint: string; status: 'Open'; publicName?: string; }
export type ItemStatus = 'Open' | 'Returned' | 'Handed to Police' | 'Donated' | 'Disposed' | 'Withdrawn';
export interface Item extends Omit<PublicItem, 'status'> {
  status: ItemStatus;
  description?: string;
  identifyingDetails?: string;
  receivedAt?: string;
  color?: string;
  brand?: string;
  building?: string;
  creatorId?: string;
  photoUrl?: string;
  canClaim?: boolean;
  hasApprovedClaim?: boolean;
  holdingPeriodEnd?: string;
  correctionRevision?: number;
}

export interface ItemHistoryEntry {
  source?: 'item' | 'claim';
  sequence: number;
  action: string;
  actor: string;
  timestamp: string;
  recordId?: string;
  reason?: string;
  details?: {
    fromHoldingPoint?: string;
    holdingPoint?: string;
    kind?: string;
    date?: string;
    decisionBasis?: string;
    policeReference?: string;
    attemptedAt?: string;
    method?: string;
    outcome?: string;
    reason?: string;
    before?: Intake;
    after?: string | Intake;
    releasedAt?: string;
    receiptReference?: string;
    verification?: string;
    checks?: { detailsCheck: string; identityCheck: string };
  };
}
export interface Intake {
  category: string;
  publicName?: string;
  description: string;
  location: string;
  dateFound: string;
  receivedAt: string;
  holdingPoint: string;
  identifyingDetails: string;
  color: string;
  brand: string;
  building: string;
}
