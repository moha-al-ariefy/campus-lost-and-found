import { DatePipe } from '@angular/common';
import { Component, input, OnInit, output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { en } from './en';
import { ApiError } from './submission';

interface ReleaseItem {
  claimId: string;
  itemId: string;
  category: string;
  publicName: string;
  location: string;
  holdingPoint: string;
}

interface ReleaseDetails { item: ReleaseItem; claimantName: string; }

interface HandoverReceipt {
  itemId: string;
  releasedAt: string;
  receiptReference: string;
}

@Component({ selector: 'app-handovers', imports: [DatePipe, FormsModule], templateUrl: './handovers.html' })
export class HandoversView implements OnInit {
  readonly request = input.required<<T>(path: string, options?: RequestInit) => Promise<T>>();
  readonly staffName = input.required<string>();
  readonly backToClaims = output<void>();
  readonly t = en;
  readonly mode = signal<'queue' | 'release' | 'confirmation'>('queue');
  readonly items = signal<ReleaseItem[]>([]);
  readonly selected = signal<ReleaseDetails | null>(null);
  readonly receipt = signal<HandoverReceipt | null>(null);
  readonly page = signal(1);
  readonly hasMore = signal(false);
  readonly busy = signal(false);
  readonly uncertain = signal(false);
  readonly error = signal('');
  readonly fields = signal<string[]>([]);
  releaseDate = '';
  releaseTime = '';
  verification = '';
  receiptReference = '';
  signedAcknowledgment = false;
  private selectedClaim = '';
  private draft: { key: string; body: string } | null = null;

  ngOnInit(): void { void this.load(1); }

  itemName(item: ReleaseItem): string {
    return item.publicName || (en.categories[item.category as keyof typeof en.categories] ?? item.category);
  }

  holdingPoint(id: string): string {
    return en.points[id as keyof typeof en.points] ?? id;
  }

  private message(error: unknown): string {
    return error instanceof ApiError ? en.errors[error.code as keyof typeof en.errors] ?? en.errors.unavailable : en.errors.network;
  }

  async load(page: number): Promise<void> {
    if (this.busy() || this.uncertain()) return;
    this.mode.set('queue'); this.error.set(''); this.busy.set(true);
    this.selected.set(null); this.receipt.set(null);
    try {
      const result = await this.request()<{ items: ReleaseItem[]; hasMore: boolean }>(`/handovers?page=${page}`);
      this.items.set(result.items); this.page.set(page); this.hasMore.set(result.hasMore);
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  async open(claimId: string): Promise<void> {
    if (this.busy() || this.uncertain()) return;
    this.selectedClaim = claimId;
    this.selected.set(null); this.error.set(''); this.fields.set([]);
    this.mode.set('release'); this.busy.set(true);
    try {
      this.selected.set(await this.request()<ReleaseDetails>(`/claims/${encodeURIComponent(claimId)}/handover`));
      const dubaiNow = new Date(Date.now() + 4 * 60 * 60 * 1000).toISOString();
      this.releaseDate = dubaiNow.slice(0, 10); this.releaseTime = dubaiNow.slice(11, 16);
      this.verification = ''; this.receiptReference = ''; this.signedAcknowledgment = false;
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  retryDetails(): void { void this.open(this.selectedClaim); }

  async submit(): Promise<void> {
    if (this.busy() || !this.selected()) return;
    this.error.set(''); this.fields.set([]); this.busy.set(true);
    this.draft ??= { key: crypto.randomUUID(), body: JSON.stringify({
      releasedAt: `${this.releaseDate}T${this.releaseTime}`, verification: this.verification,
      receiptReference: this.receiptReference, signedAcknowledgment: this.signedAcknowledgment
    }) };
    try {
      const { handover } = await this.request()<{ handover: HandoverReceipt }>(`/claims/${encodeURIComponent(this.selectedClaim)}/handover`, {
        method: 'POST', headers: { 'Idempotency-Key': this.draft.key }, body: this.draft.body
      });
      this.draft = null; this.uncertain.set(false); this.receipt.set(handover); this.mode.set('confirmation');
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.draft = null; this.uncertain.set(false); this.fields.set(error.fields);
        this.error.set(this.message(error));
      } else {
        this.uncertain.set(true); this.error.set(en.unknownOutcome);
      }
      setTimeout(() => document.getElementById('handover-error')?.focus());
    } finally { this.busy.set(false); }
  }
}
