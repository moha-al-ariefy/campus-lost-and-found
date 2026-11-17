import { Component, input, output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import type { Account, Item } from '../shared/types';
import { en } from './en';
import { ApiError } from './submission';

type Operation = 'custody' | 'disposition' | 'contact-attempt' | 'withdraw';

@Component({ selector: 'app-item-activity', imports: [FormsModule], templateUrl: './item-activity.html' })
export class ItemActivityView {
  readonly item = input.required<Item>();
  readonly account = input.required<Account>();
  readonly points = input.required<string[]>();
  readonly request = input.required<<T>(path: string, options?: RequestInit) => Promise<T>>();
  readonly saved = output<string>();
  readonly t = en;
  readonly operations: Record<Operation, string> = {
    custody: en.custody.transfer, disposition: en.custody.disposition, 'contact-attempt': en.custody.contact, withdraw: en.custody.withdraw
  };
  readonly dispositions = ['Handed to Police', 'Donated', 'Disposed'] as const;
  readonly operation = signal<Operation | null>(null);
  readonly busy = signal(false);
  readonly uncertain = signal(false);
  readonly error = signal('');
  readonly fields = signal<string[]>([]);
  destination = '';
  disposition: typeof this.dispositions[number] | '' = '';
  date = '';
  basis = '';
  policeReference = '';
  attemptedAt = '';
  method = '';
  outcome = '';
  reason = '';
  private draft: { key: string; body: string } | null = null;

  canTransfer(): boolean {
    const actor = this.account();
    return this.item().status === 'Open' && (actor.role === 'admin' ||
      (actor.role === 'staff' && actor.holdingPoints.includes(this.item().holdingPoint)));
  }

  pointName(key: string): string { return en.points[key as keyof typeof en.points] ?? key; }

  open(operation: Operation): void {
    this.error.set(''); this.fields.set([]);
    this.destination = ''; this.disposition = ''; this.basis = ''; this.policeReference = '';
    this.method = ''; this.outcome = ''; this.reason = '';
    const now = new Date(Date.now() + 4 * 60 * 60 * 1000).toISOString();
    this.date = now.slice(0, 10); this.attemptedAt = now.slice(0, 16);
    this.operation.set(operation);
  }

  private payload(operation: Operation): object {
    switch (operation) {
      case 'custody': return { fromHoldingPoint: this.item().holdingPoint, holdingPoint: this.destination };
      case 'disposition': return { kind: this.disposition, date: this.date, decisionBasis: this.basis,
        policeReference: this.disposition === 'Handed to Police' ? this.policeReference : '' };
      case 'contact-attempt': return { attemptedAt: this.attemptedAt, method: this.method, outcome: this.outcome };
      case 'withdraw': return { reason: this.reason };
    }
  }

  async submit(): Promise<void> {
    const operation = this.operation();
    if (!operation || this.busy()) return;
    this.error.set(''); this.fields.set([]); this.busy.set(true);
    this.draft ??= { key: crypto.randomUUID(), body: JSON.stringify(this.payload(operation)) };
    try {
      await this.request()(`/items/${encodeURIComponent(this.item().id)}/${operation}`, {
        method: 'POST', headers: { 'Idempotency-Key': this.draft.key }, body: this.draft.body
      });
      this.draft = null; this.uncertain.set(false); this.operation.set(null);
      this.saved.emit(en.custody.saved[operation]);
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.draft = null; this.uncertain.set(false); this.fields.set(error.fields);
        this.error.set(en.errors[error.code as keyof typeof en.errors] ?? en.errors.unavailable);
      } else {
        this.uncertain.set(true); this.error.set(en.unknownOutcome);
      }
      setTimeout(() => document.getElementById('item-activity-error')?.focus());
    } finally { this.busy.set(false); }
  }
}
