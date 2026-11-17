import { DatePipe } from '@angular/common';
import { afterNextRender, Component, effect, inject, Injector, input, output, signal, untracked } from '@angular/core';
import { FormsModule } from '@angular/forms';
import type { Account, Item, PublicItem } from '../shared/types';
import { en } from './en';
import { ApiError } from './submission';
import { HandoversView } from './handovers';

type ClaimStatus = keyof typeof en.claims.status;
interface ClaimReceipt { id: string; itemId: string; status: ClaimStatus; reason?: string; decidedAt?: string; reviewRequestedAt?: string; }
interface ClaimSummary extends ClaimReceipt { category: string; publicName: string; holdingPoint: string; createdAt: string; }
interface ClaimDetail extends ClaimSummary { descriptionAnswer: string; lossAnswer: string; contact: string; reopenedAt?: string; ownClaim?: boolean; itemStatus?: string; }
interface ClaimAnswers { descriptionAnswer: string; lossAnswer: string; contact: string; }
const blankAnswers: ClaimAnswers = { descriptionAnswer: '', lossAnswer: '', contact: '' };
type CheckResult = keyof typeof en.review.checkState;
interface VerificationChecks { detailsCheck: CheckResult; identityCheck: CheckResult; }
interface ClaimEvidence { claim: ClaimDetail; item: Item; checks: VerificationChecks; reviewer: string; }
interface DecisionForm extends VerificationChecks { outcome: '' | 'approve' | 'reject'; reason: string; }
const blankDecision: DecisionForm = { outcome: '', reason: '', detailsCheck: 'Not recorded', identityCheck: 'Not recorded' };
type ClaimCommand = 'decision' | 'review-request' | 'reopen';

@Component({ selector: 'app-claims', imports: [DatePipe, FormsModule, HandoversView], templateUrl: './claims.html' })
export class ClaimsView {
  readonly active = input.required<boolean>();
  readonly reviewEnabled = input.required<boolean>();
  readonly reopeningEnabled = input.required<boolean>();
  readonly handoversEnabled = input.required<boolean>();
  readonly account = input.required<Account>();
  readonly item = input.required<PublicItem | null>();
  readonly request = input.required<<T>(path: string, options?: RequestInit) => Promise<T>>();
  readonly openDirectory = output<void>();
  readonly t = en;
  readonly mode = signal<'list' | 'form' | 'detail' | 'handover'>('list');
  readonly claims = signal<ClaimSummary[]>([]);
  readonly target = signal<PublicItem | null>(null);
  readonly receipt = signal<ClaimReceipt | null>(null);
  readonly detail = signal<ClaimDetail | null>(null);
  readonly evidence = signal<ClaimEvidence | null>(null);
  readonly evidencePhotoFailed = signal(false);
  readonly commandNotice = signal('');
  readonly uncertainCommand = signal<ClaimCommand | null>(null);
  readonly commandError = signal('');
  readonly checkResults: CheckResult[] = ['Not recorded', 'Passed', 'Failed'];
  readonly busy = signal(false);
  readonly uncertain = signal(false);
  readonly error = signal('');
  readonly fields = signal<string[]>([]);
  readonly page = signal(1);
  readonly hasMore = signal(false);
  answers = { ...blankAnswers };
  decision = { ...blankDecision };
  reopeningReason = '';
  private openedItem: PublicItem | null | undefined;
  private selectedClaim = '';
  private draft: { key: string; body: string } | null = null;
  private commandDraft: { command: ClaimCommand; key: string; body: string } | null = null;
  private readonly injector = inject(Injector);

  constructor() {
    effect(() => {
      const item = this.item();
      if (!this.active()) return;
      untracked(() => {
        if (this.busy() || this.uncertain() || this.uncertainCommand() || item === this.openedItem) return;
        this.openedItem = item;
        if (item) {
          this.target.set(item); this.answers = { ...blankAnswers }; this.fields.set([]); this.error.set('');
          this.receipt.set(null); this.draft = null; this.mode.set('form');
          this.focusWhenActive('claim-form-title');
        } else {
          void this.load(1);
        }
      });
    });
  }

  private focusWhenActive(id: string): void {
    afterNextRender(() => {
      if (this.active()) document.getElementById(id)?.focus();
    }, { injector: this.injector });
  }

  itemName(item: { publicName?: string; category: string }): string {
    return item.publicName || (this.t.categories[item.category as keyof typeof en.categories] ?? item.category);
  }
  holdingPoint(id: string): string { return this.t.points[id as keyof typeof en.points] ?? id; }
  fieldName(id: string): string { return this.t.claims[id as keyof ClaimAnswers] ?? id; }
  private message(error: unknown): string {
    return error instanceof ApiError ? this.t.errors[error.code as keyof typeof en.errors] ?? this.t.errors.unavailable : this.t.errors.network;
  }

  async load(page: number): Promise<void> {
    if (this.busy() || this.uncertainCommand()) return;
    this.receipt.set(null); this.commandNotice.set(''); this.error.set(''); this.mode.set('list'); this.busy.set(true);
    try {
      const result = await this.request()<{ claims: ClaimSummary[]; hasMore: boolean }>(`/claims?page=${page}`);
      this.claims.set(result.claims); this.page.set(page); this.hasMore.set(result.hasMore);
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  async submit(): Promise<void> {
    const item = this.target();
    if (!item || this.busy()) return;
    this.error.set(''); this.fields.set([]); this.busy.set(true);
    this.draft ??= { key: crypto.randomUUID(), body: JSON.stringify({ itemId: item.id, ...this.answers }) };
    try {
      const result = await this.request()<{ claim: ClaimReceipt }>('/claims', { method: 'POST',
        headers: { 'Idempotency-Key': this.draft.key }, body: this.draft.body });
      this.draft = null; this.uncertain.set(false); this.answers = { ...blankAnswers };
      this.receipt.set(result.claim);
      this.focusWhenActive('claim-confirmation');
      await this.open(result.claim.id);
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.fields.set(error.fields); this.draft = null; this.uncertain.set(false);
        this.error.set(this.message(error));
      } else { this.uncertain.set(true); this.error.set(this.t.unknownOutcome); }
      setTimeout(() => document.getElementById('claim-form-error')?.focus());
    } finally { this.busy.set(false); }
  }

  async open(id: string): Promise<void> {
    if (this.selectedClaim !== id) this.commandNotice.set('');
    this.selectedClaim = id;
    this.detail.set(null); this.evidence.set(null); this.evidencePhotoFailed.set(false);
    this.reopeningReason = '';
    this.error.set(''); this.commandError.set(''); this.mode.set('detail'); this.busy.set(true);
    try {
      const path = `/claims/${encodeURIComponent(id)}`;
      if (this.reviewEnabled() && this.account().role === 'admin') {
        const evidence = await this.request()<ClaimEvidence>(`${path}/evidence`);
        this.evidence.set(evidence); this.detail.set(evidence.claim);
        this.decision = { ...blankDecision, ...evidence.checks };
      } else {
        const { claim } = await this.request()<{ claim: ClaimDetail }>(path);
        this.detail.set(claim);
      }
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  retryDetail(): void { void this.open(this.selectedClaim); }

  evidencePhoto(item: Item): string | undefined {
    const photo = `/api/items/${encodeURIComponent(item.id)}/photo`;
    return item.photoUrl === photo ? photo : undefined;
  }

  saveDecision(): void {
    const current = this.evidence();
    if (current?.claim.status !== 'Submitted') return;
    void this.sendCommand(current.claim.id, 'decision', { ...this.decision, reopenedAt: current.claim.reopenedAt }, this.t.review.saved);
  }

  requestReview(): void {
    const claim = this.detail();
    if (!claim?.ownClaim || claim.status !== 'Rejected' || claim.reviewRequestedAt) return;
    void this.sendCommand(claim.id, 'review-request', { decidedAt: claim.decidedAt }, this.t.review.requested);
  }

  reopen(): void {
    const current = this.evidence();
    if (current?.claim.status !== 'Rejected') return;
    void this.sendCommand(current.claim.id, 'reopen', { decidedAt: current.claim.decidedAt, reason: this.reopeningReason }, this.t.review.reopened);
  }

  private async sendCommand(id: string, command: ClaimCommand, body: object, notice: string): Promise<void> {
    if (this.busy() || (this.commandDraft && this.commandDraft.command !== command)) return;
    this.commandNotice.set(''); this.commandError.set(''); this.busy.set(true);
    this.commandDraft ??= { command, key: crypto.randomUUID(), body: JSON.stringify(body) };
    try {
      const { claim } = await this.request()<{ claim: ClaimReceipt }>(`/claims/${encodeURIComponent(id)}/${command}`, {
        method: 'POST', headers: { 'Idempotency-Key': this.commandDraft.key }, body: this.commandDraft.body
      });
      this.commandDraft = null; this.uncertainCommand.set(null); this.commandNotice.set(notice);
      await this.open(claim.id);
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.commandDraft = null; this.uncertainCommand.set(null); this.commandError.set(this.message(error));
      } else {
        this.uncertainCommand.set(command); this.commandError.set(this.t.unknownOutcome);
      }
      setTimeout(() => document.getElementById('claim-command-error')?.focus());
    } finally { this.busy.set(false); }
  }
}
