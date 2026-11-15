export type Role = 'campus' | 'staff' | 'admin';
export interface Account { id: string; name: string; role: Role; holdingPoints: string[]; }
export interface PublicItem { id: string; category: string; location: string; dateFound: string; holdingPoint: string; status: 'Open'; }
export interface Item extends PublicItem {
  description?: string;
  identifyingDetails?: string;
  receivedAt?: string;
  color?: string;
  brand?: string;
  building?: string;
  creatorId?: string;
}
export interface Intake {
  category: string;
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
