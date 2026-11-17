import { bootstrapApplication } from '@angular/platform-browser';
import { afterNextRender, Component, inject, Injector, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import type { Account, Intake, Item, ItemHistoryEntry, PublicItem } from '../shared/types';
import { en } from './en';
import { ApiError, photoFileError, photoSubmissionRequest } from './submission';
import { LostReportsView } from './lost-reports';
import { ClaimsView } from './claims';
import { ItemActivityView } from './item-activity';

interface IntakeSubmission {
  key: string;
  data: Intake;
  photo: File | null;
}

const emptyFilters = { publicName: '', category: '', location: '', holdingPoint: '', from: '', to: '' };

@Component({
  selector: 'app-root', standalone: true, imports: [FormsModule, DatePipe, LostReportsView, ClaimsView, ItemActivityView],
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
  readonly history = signal<ItemHistoryEntry[]>([]);
  readonly view = signal<'directory' | 'intake' | 'reports' | 'claims'>('directory');
  readonly lostReportsEnabled = signal(false);
  readonly claimsEnabled = signal(false);
  readonly claimReviewEnabled = signal(false);
  readonly claimReopeningEnabled = signal(false);
  readonly handoversEnabled = signal(false);
  readonly custodyEnabled = signal(false);
  readonly correction = signal<{ id: string; revision: number } | null>(null);
  readonly claimItem = signal<PublicItem | null>(null);
  readonly uncertain = signal(false);
  readonly photoPreview = signal<{ name: string; url: string } | null>(null);
  readonly photoError = signal('');
  readonly photoReadFailed = signal(false);
  readonly categories = signal<string[]>([]);
  readonly points = signal<string[]>([]);
  readonly itemNamesEnabled = signal(false);
  loginName = '';
  displayName = '';
  password = '';
  filters = { ...emptyFilters };
  recordReference = '';
  form: Intake = this.blankForm();
  private intakePhoto: File | null = null;
  private submission: IntakeSubmission | null = null;
  private queryVersion = 0;
  private detailVersion = 0;
  private selectedId = '';
  private activeFilters = { ...this.filters };
  private readonly injector = inject(Injector);
  private detailOpener: HTMLElement | null = null;
  private directoryScroll = 0;

  constructor() { void this.restoreSession(); }

  readonly request = <T>(path: string, options?: RequestInit): Promise<T> => this.api<T>(path, options);

  categoryName(key: string): string { return this.t.categories[key as keyof typeof en.categories] ?? key; }
  itemTitle(item: Pick<PublicItem, 'publicName' | 'category'>): string { return item.publicName || this.categoryName(item.category); }
  historyAction(action: string): string { return this.t.custody.events[action as keyof typeof en.custody.events] ?? this.t.custody.event; }
  correctionChanges(entry: ItemHistoryEntry): { field: keyof Intake; before: string; after: string }[] {
    const { before, after } = entry.details ?? {};
    if (!before || !after || typeof after !== 'object') return [];
    return (Object.keys(after) as (keyof Intake)[]).filter(field => (before[field] ?? '') !== (after[field] ?? ''))
      .map(field => ({ field, before: before[field] || this.t.notProvided, after: after[field] || this.t.notProvided }));
  }
  pointName(key: string): string { return this.t.points[key as keyof typeof en.points] ?? key; }
  fieldName(key: string): string { return String(this.t[key as keyof typeof en] ?? key); }
  initials(): string { return this.account()?.name.split(' ').map(part => part[0]).slice(0, 2).join('') ?? ''; }
  allowedPoints(): string[] { return this.account()?.role === 'admin' ? this.points() : this.account()?.holdingPoints ?? []; }
  hasFilters(): boolean { return Object.values(this.activeFilters).some(Boolean); }
  appliedExtraFilters(): number {
    const { category, holdingPoint, from, to, location } = this.activeFilters;
    return [category, holdingPoint, from, to, this.itemNamesEnabled() ? location : ''].filter(Boolean).length;
  }
  message(error: unknown): string {
    return error instanceof ApiError ? this.t.errors[error.code as keyof typeof en.errors] ?? this.t.errors.unavailable : this.t.errors.network;
  }

  private async api<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers);
    headers.set('X-Campus-Request', '1');
    if (typeof options.body === 'string' && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json');
    }
    let response: Response;
    try {
      response = await fetch('/api' + path, { ...options, credentials: 'same-origin', signal: AbortSignal.timeout(10000),
        headers });
    } catch { throw new ApiError('network'); }
    const data = await response.json().catch(() => ({ code: 'unavailable' }));
    if (!response.ok) {
      if (response.status === 401 && path !== '/login') {
        this.account.set(null); this.items.set([]); this.selected.set(null); this.history.set([]);
        this.resetIntake();
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
      this.claimItem.set(null);
      this.resetIntake(); this.success.set('');
      this.filters = { ...emptyFilters };
    } catch (error) { this.error.set(this.message(error)); }
    finally { this.busy.set(false); }
  }

  async loadConfig(): Promise<void> {
    const config = await this.api<{ categories: string[]; holdingPoints: string[]; itemNames?: boolean; lostReports?: boolean; claims?: boolean; claimReview?: boolean; claimReopening?: boolean; handovers?: boolean; custody?: boolean }>('/config');
    this.categories.set(config.categories); this.points.set(config.holdingPoints);
    this.itemNamesEnabled.set(config.itemNames === true);
    this.lostReportsEnabled.set(config.lostReports === true);
    this.claimsEnabled.set(config.claims === true);
    this.claimReviewEnabled.set(config.claimReview === true);
    this.claimReopeningEnabled.set(config.claimReopening === true);
    this.handoversEnabled.set(config.handovers === true);
    this.custodyEnabled.set(config.custody === true);
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

  clearFilters(): void { this.filters = { ...emptyFilters }; void this.search(); }

  async openItem(id: string, opener?: EventTarget | null): Promise<void> {
    const version = ++this.detailVersion;
    if (opener instanceof HTMLElement) {
      this.detailOpener = opener;
      this.directoryScroll = window.scrollY;
    }
    this.selectedId = id; this.detailLoading.set(true); this.detailError.set(''); this.selected.set(null); this.history.set([]);
    this.photoReadFailed.set(false);
    afterNextRender(() => {
      if (version !== this.detailVersion) return;
      const panel = document.getElementById('item-details');
      panel?.focus({ preventScroll: true });
      panel?.scrollIntoView({ block: 'nearest' });
    }, { injector: this.injector });
    try {
      const result = await this.api<{ item: Item }>('/items/' + encodeURIComponent(id));
      if (version !== this.detailVersion) return;
      this.selected.set(result.item);
      if (this.account()?.role === 'admin') {
        const result = await this.api<{ entries: ItemHistoryEntry[] }>('/items/' + encodeURIComponent(id) + '/history');
        if (version === this.detailVersion) this.history.set(result.entries);
      }
    } catch (error) { if (version === this.detailVersion) this.detailError.set(this.message(error)); }
    finally { if (version === this.detailVersion) this.detailLoading.set(false); }
  }

  retryDetail(): void { void this.openItem(this.selectedId); }
  openReferencedItem(event: Event): void {
    const id = this.recordReference.trim();
    if (id && this.account()?.role === 'admin') void this.openItem(id, event instanceof SubmitEvent ? event.submitter : null);
  }
  itemActivitySaved(id: string, message: string): void {
    if (this.view() !== 'directory' || this.selected()?.id !== id) return;
    this.success.set(message);
    void this.openItem(id);
    void this.search(this.page(), true);
  }
  closeDetail(restoreFocus = true): void {
    const version = ++this.detailVersion;
    const opener = this.detailOpener;
    const scrollPosition = this.directoryScroll;
    this.detailOpener = null;
    this.directoryScroll = 0;
    this.selected.set(null); this.detailError.set(''); this.detailLoading.set(false);
    if (!restoreFocus) return;
    afterNextRender(() => {
      if (version !== this.detailVersion) return;
      const target = opener?.isConnected ? opener : document.getElementById('directory-search');
      target?.focus({ preventScroll: true });
      window.scrollTo(0, scrollPosition);
    }, { injector: this.injector });
  }
  showDirectory(): void { this.view.set('directory'); this.error.set(''); this.closeDetail(false); void this.search(); }
  showReports(): void { this.view.set('reports'); this.error.set(''); this.success.set(''); this.closeDetail(false); }
  showClaims(): void { this.claimItem.set(null); this.view.set('claims'); this.error.set(''); this.success.set(''); this.closeDetail(false); }
  beginClaim(item: Item): void {
    if (item.status !== 'Open') return;
    this.showClaims();
    this.claimItem.set({ ...item, status: 'Open' });
  }
  showSuggestedItem(id: string): void { this.showDirectory(); void this.openItem(id); }
  showIntake(): void {
    this.view.set('intake'); this.success.set(''); this.error.set(''); this.closeDetail(false);
    if (!this.submission && !this.form.category) { this.form = this.blankForm(); this.form.holdingPoint = this.allowedPoints()[0] ?? ''; }
  }
  beginCorrection(item: Item): void {
    if (this.busy() || this.uncertain() || item.correctionRevision === undefined) return;
    this.resetIntake();
    this.showIntake();
    this.correction.set({ id: item.id, revision: item.correctionRevision });
    this.form = {
      category: item.category, publicName: item.publicName ?? '', description: item.description ?? '',
      location: item.location, dateFound: item.dateFound, receivedAt: item.receivedAt ?? '',
      holdingPoint: item.holdingPoint, identifyingDetails: item.identifyingDetails ?? '',
      color: item.color ?? '', brand: item.brand ?? '', building: item.building ?? ''
    };
  }
  cancelIntake(): void {
    if (this.correction()) this.resetIntake();
    this.showDirectory();
  }
  private blankForm(): Intake {
    const now = new Date(Date.now() + 4 * 60 * 60 * 1000).toISOString();
    return { category: '', description: '', location: '', dateFound: now.slice(0, 10), receivedAt: now.slice(0, 16), holdingPoint: '', identifyingDetails: '', color: '', brand: '', building: '' };
  }

  selectPhoto(input: HTMLInputElement): void {
    const file = input.files?.item(0);
    input.value = '';
    if (!file || this.busy() || this.uncertain()) return;
    this.removePhoto();
    const error = photoFileError(file);
    if (error) {
      this.photoError.set(this.t.errors[error]);
      return;
    }
    this.intakePhoto = file;
    this.photoPreview.set({ name: file.name, url: URL.createObjectURL(file) });
  }

  removePhoto(): void {
    const preview = this.photoPreview();
    if (preview) URL.revokeObjectURL(preview.url);
    this.intakePhoto = null;
    this.photoPreview.set(null);
    this.photoError.set('');
  }

  itemPhotoUrl(item: Item): string | null {
    const path = '/api/items/' + encodeURIComponent(item.id) + '/photo';
    return item.photoUrl === path ? path : null;
  }

  private resetIntake(): void {
    this.correction.set(null);
    this.form = this.blankForm();
    this.submission = null;
    this.uncertain.set(false);
    this.removePhoto();
  }

  async save(): Promise<void> {
    if (this.busy() || this.photoError()) return;
    this.busy.set(true); this.error.set(''); this.invalidFields.set([]);
    if (!this.submission) this.submission = { key: crypto.randomUUID(), data: { ...this.form }, photo: this.intakePhoto };
    try {
      const { key, data, photo } = this.submission;
      const correction = this.correction();
      let id: string;
      if (correction) {
        const result = await this.api<{ activity: { itemId: string } }>(`/items/${encodeURIComponent(correction.id)}/correction`, {
          method: 'POST', headers: { 'Idempotency-Key': key }, body: JSON.stringify({ ...data, revision: correction.revision })
        });
        id = result.activity.itemId;
      } else {
        const result = await this.api<{ item: Item }>('/items', await photoSubmissionRequest(key, data, photo));
        id = result.item.id;
      }
      this.resetIntake();
      this.success.set(correction ? this.t.custody.corrected : this.t.saved); this.view.set('directory');
      await this.search(); await this.openItem(id);
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.invalidFields.set(error.fields); this.error.set(this.message(error)); this.submission = null; this.uncertain.set(false);
      } else { this.uncertain.set(true); this.error.set(this.t.unknownOutcome); }
      setTimeout(() => document.getElementById('form-errors')?.focus());
    } finally { this.busy.set(false); }
  }
}

bootstrapApplication(App).catch(error => console.error(error));
