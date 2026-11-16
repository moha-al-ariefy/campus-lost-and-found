import { bootstrapApplication } from '@angular/platform-browser';
import { Component, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import type { Account, Intake, Item, PublicItem } from '../shared/types';
import { en } from './en';

class ApiError extends Error {
  constructor(public code: string, public status = 0, public fields: string[] = []) { super(code); }
}

@Component({
  selector: 'app-root', standalone: true, imports: [FormsModule, DatePipe],
  templateUrl: './app.html'
})
export class App {
  readonly t = en;
  readonly account = signal<Account | null>(null);
  readonly booting = signal(true);
  readonly needsSetup = signal(false);
  readonly signupEnabled = signal(false);
  readonly signingUp = signal(false);
  readonly setupMessage = signal('');
  readonly busy = signal(false);
  readonly loading = signal(false);
  readonly detailLoading = signal(false);
  readonly error = signal('');
  readonly listError = signal('');
  readonly detailError = signal('');
  readonly success = signal('');
  readonly invalidFields = signal<string[]>([]);
  readonly items = signal<PublicItem[]>([]);
  readonly total = signal(0);
  readonly page = signal(1);
  readonly selected = signal<Item | null>(null);
  readonly history = signal<{ sequence: number; action: string; timestamp: string; actor: string }[]>([]);
  readonly view = signal<'directory' | 'intake'>('directory');
  readonly uncertain = signal(false);
  readonly categories = signal<string[]>([]);
  readonly points = signal<string[]>([]);
  loginName = '';
  displayName = '';
  password = '';
  filters = { category: '', location: '', holdingPoint: '', from: '', to: '' };
  form: Intake = this.blankForm();
  private submission: { key: string; data: Intake } | null = null;
  private queryVersion = 0;
  private detailVersion = 0;
  private selectedId = '';
  private activeFilters = { ...this.filters };

  constructor() { void this.restoreSession(); }

  categoryName(key: string): string { return this.t.categories[key as keyof typeof en.categories] ?? key; }
  pointName(key: string): string { return this.t.points[key as keyof typeof en.points] ?? key; }
  fieldName(key: string): string { return String(this.t[key as keyof typeof en] ?? key); }
  initials(): string { return this.account()?.name.split(' ').map(part => part[0]).slice(0, 2).join('') ?? ''; }
  allowedPoints(): string[] { return this.account()?.role === 'admin' ? this.points() : this.account()?.holdingPoints ?? []; }
  hasFilters(): boolean { return Object.values(this.activeFilters).some(Boolean); }
  message(error: unknown): string {
    return error instanceof ApiError ? this.t.errors[error.code as keyof typeof en.errors] ?? this.t.errors.unavailable : this.t.errors.network;
  }

  private async api<T>(path: string, options: RequestInit = {}): Promise<T> {
    let response: Response;
    try {
      response = await fetch('/api' + path, { ...options, credentials: 'same-origin', signal: AbortSignal.timeout(10000),
        headers: { 'Content-Type': 'application/json', 'X-Campus-Request': '1', ...options.headers } });
    } catch { throw new ApiError('network'); }
    const data = await response.json().catch(() => ({ code: 'unavailable' }));
    if (!response.ok) {
      if (response.status === 401 && path !== '/login') {
        this.account.set(null); this.items.set([]); this.selected.set(null); this.history.set([]);
        this.form = this.blankForm(); this.submission = null; this.uncertain.set(false);
        this.detailVersion++; this.queryVersion++;
        this.error.set(this.t.errors.sessionExpired);
      }
      throw new ApiError(data.code, response.status, data.fields);
    }
    return data as T;
  }

  async restoreSession(): Promise<void> {
    try {
      const setup = await this.api<{ needsSetup: boolean; signupEnabled?: boolean }>('/setup');
      this.needsSetup.set(setup.needsSetup);
      this.signupEnabled.set(setup.signupEnabled === true);
      if (setup.needsSetup) return;
      const session = await this.api<{ account: Account }>('/session');
      this.account.set(session.account); await this.loadConfig(); await this.search();
    } catch (error) { if (!(error instanceof ApiError && error.status === 401)) this.error.set(this.message(error)); else this.error.set(''); }
    finally { this.booting.set(false); }
  }

  async signIn(): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true); this.error.set('');
    try {
      if (this.needsSetup()) {
        await this.api('/setup', { method: 'POST', body: JSON.stringify({ password: this.password }) });
        this.needsSetup.set(false); this.setupMessage.set(this.t.setupDone); this.password = ''; return;
      }
      if (this.signingUp()) {
        await this.api('/signup', {
          method: 'POST',
          body: JSON.stringify({ name: this.displayName, login: this.loginName, password: this.password }),
        });
        this.showSignup(false);
        this.setupMessage.set(this.t.signupDone);
        return;
      }
      const session = await this.api<{ account: Account }>('/login', { method: 'POST', body: JSON.stringify({ login: this.loginName, password: this.password }) });
      this.password = ''; this.account.set(session.account); this.view.set('directory');
      await this.loadConfig(); await this.search();
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  showSignup(visible: boolean): void {
    if (visible && !this.signupEnabled()) return;
    this.signingUp.set(visible);
    this.password = '';
    this.error.set('');
    this.setupMessage.set('');
  }

  async signOut(): Promise<void> {
    this.busy.set(true); this.error.set('');
    try {
      await this.api('/logout', { method: 'POST', body: '{}' });
      this.queryVersion++; this.detailVersion++;
      this.account.set(null); this.items.set([]); this.selected.set(null); this.history.set([]);
      this.form = this.blankForm(); this.submission = null; this.uncertain.set(false); this.success.set('');
      this.filters = { category: '', location: '', holdingPoint: '', from: '', to: '' };
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  async loadConfig(): Promise<void> {
    const config = await this.api<{ categories: string[]; holdingPoints: string[] }>('/config');
    this.categories.set(config.categories); this.points.set(config.holdingPoints);
  }

  async search(page = 1, preserveFilters = false): Promise<void> {
    const version = ++this.queryVersion;
    this.loading.set(true); this.listError.set('');
    if (!preserveFilters) this.activeFilters = { ...this.filters };
    const params = new URLSearchParams({ page: String(page) });
    for (const [key, value] of Object.entries(this.activeFilters)) if (value.trim()) params.set(key, value.trim());
    try {
      const result = await this.api<{ items: PublicItem[]; total: number; page: number }>('/items?' + params);
      if (version !== this.queryVersion) return;
      this.items.set(result.items); this.total.set(result.total); this.page.set(result.page);
    } catch (error) { if (version === this.queryVersion) { this.items.set([]); this.listError.set(this.message(error)); } }
    finally { if (version === this.queryVersion) this.loading.set(false); }
  }

  clearFilters(): void { this.filters = { category: '', location: '', holdingPoint: '', from: '', to: '' }; void this.search(); }

  async openItem(id: string): Promise<void> {
    const version = ++this.detailVersion;
    this.selectedId = id; this.detailLoading.set(true); this.detailError.set(''); this.selected.set(null); this.history.set([]);
    try {
      const result = await this.api<{ item: Item }>('/items/' + encodeURIComponent(id));
      if (version !== this.detailVersion) return;
      this.selected.set(result.item);
      if (this.account()?.role === 'admin') {
        const result = await this.api<{ entries: { sequence: number; action: string; timestamp: string; actor: string }[] }>('/items/' + encodeURIComponent(id) + '/history');
        if (version === this.detailVersion) this.history.set(result.entries);
      }
    } catch (error) { if (version === this.detailVersion) this.detailError.set(this.message(error)); }
    finally { if (version === this.detailVersion) this.detailLoading.set(false); }
  }

  retryDetail(): void { void this.openItem(this.selectedId); }
  closeDetail(): void { this.detailVersion++; this.selected.set(null); this.detailError.set(''); this.detailLoading.set(false); }
  showDirectory(): void { this.view.set('directory'); this.error.set(''); this.closeDetail(); void this.search(); }
  showIntake(): void {
    this.view.set('intake'); this.success.set(''); this.error.set(''); this.closeDetail();
    if (!this.submission && !this.form.category) { this.form = this.blankForm(); this.form.holdingPoint = this.allowedPoints()[0] ?? ''; }
  }
  private blankForm(): Intake {
    const now = new Date(Date.now() + 4 * 60 * 60 * 1000).toISOString();
    return { category: '', description: '', location: '', dateFound: now.slice(0, 10), receivedAt: now.slice(0, 16), holdingPoint: '', identifyingDetails: '', color: '', brand: '', building: '' };
  }

  async save(): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true); this.error.set(''); this.invalidFields.set([]);
    if (!this.submission) this.submission = { key: crypto.randomUUID(), data: { ...this.form } };
    try {
      const result = await this.api<{ item: Item }>('/items', { method: 'POST', headers: { 'Idempotency-Key': this.submission.key }, body: JSON.stringify(this.submission.data) });
      this.submission = null; this.uncertain.set(false); this.form = this.blankForm();
      this.success.set(this.t.saved); this.view.set('directory');
      await this.search(); await this.openItem(result.item.id);
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.invalidFields.set(error.fields); this.error.set(this.message(error)); this.submission = null; this.uncertain.set(false);
      } else { this.uncertain.set(true); this.error.set(this.t.unknownOutcome); }
      setTimeout(() => document.getElementById('form-errors')?.focus());
    } finally { this.busy.set(false); }
  }
}

bootstrapApplication(App).catch(error => console.error(error));
