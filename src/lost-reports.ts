import { DatePipe } from '@angular/common';
import { Component, effect, input, OnDestroy, output, signal, untracked } from '@angular/core';
import { FormsModule } from '@angular/forms';
import type { Account, PublicItem } from '../shared/types';
import { en } from './en';
import { ApiError, photoFileError, photoSubmissionRequest } from './submission';

interface ReportInput {
  category: string; description: string; dateLost: string;
  color: string; brand: string; building: string; contact: string; assisted: boolean;
}
interface ReportSummary {
  id: string; reporterId: string; category: string; dateLost: string; assisted: boolean;
}
interface Report extends ReportSummary, ReportInput { photoUrl?: string; }

@Component({
  selector: 'app-lost-reports', imports: [FormsModule, DatePipe],
  templateUrl: './lost-reports.html'
})
export class LostReportsView implements OnDestroy {
  readonly active = input.required<boolean>();
  readonly account = input.required<Account>();
  readonly categories = input.required<string[]>();
  readonly request = input.required<<T>(path: string, options?: RequestInit) => Promise<T>>();
  readonly viewItem = output<string>();
  readonly t = en;
  readonly mode = signal<'list' | 'form' | 'detail'>('list');
  readonly reports = signal<ReportSummary[]>([]);
  readonly report = signal<Report | null>(null);
  readonly matches = signal<PublicItem[]>([]);
  readonly busy = signal(false);
  readonly loadingMatches = signal(false);
  readonly error = signal('');
  readonly matchError = signal('');
  readonly success = signal('');
  readonly invalid = signal<string[]>([]);
  readonly uncertain = signal(false);
  readonly preview = signal('');
  readonly fileName = signal('');
  readonly photoError = signal('');
  readonly listPage = signal(1);
  readonly matchesPage = signal(1);
  readonly moreReports = signal(false);
  readonly moreMatches = signal(false);
  readonly photoFailed = signal(false);
  form: ReportInput = this.emptyForm();
  private photo: File | null = null;
  private submission: { key: string; data: ReportInput; photo: File | null } | null = null;
  private loaded = false;
  private detailId = '';
  private matchVersion = 0;

  constructor() {
    effect(() => {
      if (this.active()) untracked(() => { if (!this.loaded) void this.loadPage(1); });
    });
  }

  ngOnDestroy(): void { this.releasePhoto(); }
  categoryName(key: string): string { return this.t.categories[key as keyof typeof en.categories] ?? key; }
  pointName(key: string): string { return this.t.points[key as keyof typeof en.points] ?? key; }
  fieldName(key: string): string {
    return this.t.reports[key as keyof typeof en.reports] ?? String(this.t[key as keyof typeof en] ?? key);
  }
  private emptyForm(): ReportInput {
    return { category: '', description: '', dateLost: '', color: '', brand: '', building: '', contact: '', assisted: false };
  }
  private message(error: unknown, fallback: string): string {
    return error instanceof ApiError ? this.t.errors[error.code as keyof typeof en.errors] ?? fallback : fallback;
  }

  async loadPage(page: number): Promise<void> {
    if (this.busy()) return;
    this.loaded = true;
    this.matchVersion++;
    this.mode.set('list'); this.error.set(''); this.success.set(''); this.busy.set(true);
    try {
      const result = await this.request()<{ reports: ReportSummary[]; hasMore: boolean }>(`/reports?page=${page}`);
      this.reports.set(result.reports); this.listPage.set(page); this.moreReports.set(result.hasMore);
    } catch (error) { this.error.set(this.message(error, this.t.errors.unavailable)); }
    finally { this.busy.set(false); }
  }

  showForm(): void {
    this.error.set(''); this.success.set(''); this.invalid.set([]); this.mode.set('form');
  }

  selectPhoto(input: HTMLInputElement): void {
    const file = input.files?.item(0);
    input.value = '';
    if (!file || this.busy() || this.uncertain()) return;
    this.releasePhoto();
    const error = photoFileError(file);
    if (error) { this.photoError.set(this.t.errors[error]); return; }
    this.photo = file;
    this.preview.set(URL.createObjectURL(file)); this.fileName.set(file.name);
  }

  releasePhoto(): void {
    if (this.preview()) URL.revokeObjectURL(this.preview());
    this.photo = null; this.preview.set(''); this.fileName.set(''); this.photoError.set('');
  }

  async save(): Promise<void> {
    if (this.busy() || this.photoError()) return;
    this.busy.set(true); this.error.set(''); this.invalid.set([]);
    this.submission ??= { key: crypto.randomUUID(), data: { ...this.form }, photo: this.photo };
    try {
      const { key, data, photo } = this.submission;
      const { report } = await this.request()<{ report: Report }>('/reports', await photoSubmissionRequest(key, data, photo));
      this.submission = null; this.uncertain.set(false); this.form = this.emptyForm(); this.releasePhoto();
      this.success.set(this.t.reports.saved); this.showDetail(report);
    } catch (error) {
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
        this.submission = null; this.uncertain.set(false); this.invalid.set(error.fields);
        this.error.set(this.message(error, this.t.errors.unavailable));
      } else {
        this.uncertain.set(true); this.error.set(this.t.unknownOutcome);
      }
      setTimeout(() => document.getElementById('report-form-error')?.focus());
    } finally { this.busy.set(false); }
  }

  async openReport(id: string): Promise<void> {
    if (this.busy()) return;
    this.detailId = id;
    this.mode.set('detail'); this.report.set(null); this.error.set(''); this.success.set(''); this.busy.set(true);
    try {
      const { report } = await this.request()<{ report: Report }>(`/reports/${encodeURIComponent(id)}`);
      this.showDetail(report);
    } catch (error) {
      this.error.set(this.message(error, this.t.reports.unavailable));
    } finally { this.busy.set(false); }
  }

  retryDetail(): void { void this.openReport(this.detailId); }

  private showDetail(report: Report): void {
    this.detailId = report.id;
    this.report.set(report); this.photoFailed.set(false); this.mode.set('detail'); this.matches.set([]);
    void this.loadMatches(1);
  }

  reportPhoto(report: Report): string | undefined {
    const path = `/api/reports/${encodeURIComponent(report.id)}/photo`;
    return report.photoUrl === path ? path : undefined;
  }

  async loadMatches(page: number): Promise<void> {
    const report = this.report();
    if (!report) return;
    const version = ++this.matchVersion;
    this.loadingMatches.set(true); this.matchError.set('');
    try {
      const result = await this.request()<{ items: PublicItem[]; hasMore: boolean }>(`/reports/${encodeURIComponent(report.id)}/matches?page=${page}`);
      if (version !== this.matchVersion) return;
      this.matches.set(result.items); this.matchesPage.set(page); this.moreMatches.set(result.hasMore);
    } catch (error) {
      if (version === this.matchVersion) this.matchError.set(this.message(error, this.t.reports.matchUnavailable));
    } finally {
      if (version === this.matchVersion) this.loadingMatches.set(false);
    }
  }
}
