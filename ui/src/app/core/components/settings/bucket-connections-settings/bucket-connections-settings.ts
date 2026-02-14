import { Component, computed, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';
import { MatTableModule } from '@angular/material/table';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { MatChipsModule, MatChipInputEvent } from '@angular/material/chips';
import {
  MatAutocompleteModule,
  MatAutocompleteSelectedEvent,
} from '@angular/material/autocomplete';

import { GlobalService } from '../../../services/global';
import { BucketConfig, BucketConfigsService } from '../../../services/bucket-configs';

type BucketDraft = BucketConfig;

@Component({
  selector: 'app-bucket-connections-settings',
  imports: [
    CommonModule,
    FormsModule,
    MatCardModule,
    MatButtonModule,
    MatIconModule,
    MatFormFieldModule,
    MatInputModule,
    MatSlideToggleModule,
    MatTableModule,
    MatSnackBarModule,
    MatChipsModule,
    MatAutocompleteModule,
  ],
  templateUrl: './bucket-connections-settings.html',
  styleUrl: './bucket-connections-settings.scss',
})
export class BucketConnectionsSettings implements OnInit {
  private readonly global = inject(GlobalService);
  private readonly bucketsService = inject(BucketConfigsService);
  private readonly snack = inject(MatSnackBar);

  readonly buckets = signal<BucketConfig[]>([]);
  readonly editingBucketId = signal<string | null>(null);

  readonly draft = signal<BucketDraft>({
    bucket_id: '',
    endpoint: '',
    access_key_id: '',
    secret_access_key: '',
    secure: true,
    bucket_name: '',
    location: '',
    authorized_users: [],
    authorized_groups: [],
  });

  readonly isEditing = computed(() => this.editingBucketId() !== null);
  readonly displayedColumns = [
    'bucket_id',
    'endpoint',
    'bucket_name',
    'location',
    'secure',
    'actions',
  ] as const;

  // Chips / autocomplete state
  readonly userChipInput = signal<string>('');
  readonly groupChipInput = signal<string>('');

  readonly allKnownUsers = computed(() => {
    const set = new Set<string>();
    for (const b of this.buckets()) for (const u of b.authorized_users ?? []) set.add(u);
    return [...set].sort((a, b) => a.localeCompare(b));
  });

  readonly allKnownGroups = computed(() => {
    const set = new Set<string>();
    for (const b of this.buckets()) for (const g of b.authorized_groups ?? []) set.add(g);
    return [...set].sort((a, b) => a.localeCompare(b));
  });

  readonly filteredUserSuggestions = computed(() => {
    const q = this.userChipInput().trim().toLowerCase();
    if (!q) return this.allKnownUsers();
    return this.allKnownUsers().filter((u) => u.toLowerCase().includes(q));
  });

  readonly filteredGroupSuggestions = computed(() => {
    const q = this.groupChipInput().trim().toLowerCase();
    if (!q) return this.allKnownGroups();
    return this.allKnownGroups().filter((g) => g.toLowerCase().includes(q));
  });

  constructor() {
    // this.global.updateTitle('Settings · Bucket Connections');
  }

  ngOnInit() {
    queueMicrotask(() => this.global.updateTitle('Settings · Bucket Connections'));
    this.refreshBuckets().then((_) => {});
  }

  private async refreshBuckets(): Promise<void> {
    try {
      const list = await this.bucketsService.listConnections();
      this.buckets.set(list ?? []);
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to load bucket connections';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
      this.buckets.set([]);
    }
  }

  startAdd(): void {
    this.editingBucketId.set(null);
    this.userChipInput.set('');
    this.groupChipInput.set('');
    this.draft.set({
      bucket_id: '',
      endpoint: '',
      access_key_id: '',
      secret_access_key: '',
      secure: true,
      bucket_name: '',
      location: '',
      authorized_users: [],
      authorized_groups: [],
    });
  }

  startEdit(row: BucketConfig): void {
    this.editingBucketId.set(row.bucket_id);
    this.userChipInput.set('');
    this.groupChipInput.set('');
    this.draft.set({ ...row });
  }

  cancel(): void {
    this.startAdd();
  }

  // --- Authorized users chips helpers ---
  addAuthorizedUserFromInput(e: MatChipInputEvent): void {
    const raw = (e.value ?? '').trim();
    if (!raw) return;

    const parts = raw
      .split(/[,\n\r\t ]+/g)
      .map((s) => s.trim())
      .filter(Boolean);

    for (const p of parts) this.addAuthorizedUser(p);

    e.chipInput?.clear();
    this.userChipInput.set('');
  }

  addAuthorizedUserFromOption(e: MatAutocompleteSelectedEvent): void {
    const value = String(e.option.value ?? '').trim();
    if (value) this.addAuthorizedUser(value);
    this.userChipInput.set('');
  }

  removeAuthorizedUser(value: string): void {
    const next = (this.draft().authorized_users ?? []).filter((u) => u !== value);
    this.draft.set({ ...this.draft(), authorized_users: next });
  }

  private addAuthorizedUser(value: string): void {
    const v = value.trim();
    if (!v) return;

    const current = this.draft().authorized_users ?? [];
    if (current.includes(v)) return;

    this.draft.set({ ...this.draft(), authorized_users: [...current, v] });
  }

  // --- Authorized groups chips helpers ---
  addAuthorizedGroupFromInput(e: MatChipInputEvent): void {
    const raw = (e.value ?? '').trim();
    if (!raw) return;

    const parts = raw
      .split(/[,\n\r\t ]+/g)
      .map((s) => s.trim())
      .filter(Boolean);

    for (const p of parts) this.addAuthorizedGroup(p);

    e.chipInput?.clear();
    this.groupChipInput.set('');
  }

  addAuthorizedGroupFromOption(e: MatAutocompleteSelectedEvent): void {
    const value = String(e.option.value ?? '').trim();
    if (value) this.addAuthorizedGroup(value);
    this.groupChipInput.set('');
  }

  removeAuthorizedGroup(value: string): void {
    const next = (this.draft().authorized_groups ?? []).filter((g) => g !== value);
    this.draft.set({ ...this.draft(), authorized_groups: next });
  }

  private addAuthorizedGroup(value: string): void {
    const v = value.trim();
    if (!v) return;

    const current = this.draft().authorized_groups ?? [];
    if (current.includes(v)) return;

    this.draft.set({ ...this.draft(), authorized_groups: [...current, v] });
  }

  async delete(bucketId: string): Promise<void> {
    try {
      await this.bucketsService.deleteConnection(bucketId);
      await this.refreshBuckets();
      this.snack.open('Bucket config deleted', 'Dismiss', { duration: 2500 });
      if (this.editingBucketId() === bucketId) this.startAdd();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Delete failed';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
    }
  }

  async save(): Promise<void> {
    const d = this.draft();
    const validationError = this.validateDraft(d);
    if (validationError) {
      this.snack.open(validationError, 'Dismiss', { duration: 3500 });
      return;
    }

    try {
      await this.bucketsService.addConnection(d);
      await this.refreshBuckets();

      const editingId = this.editingBucketId();
      this.snack.open(editingId ? 'Bucket config updated' : 'Bucket config added', 'Dismiss', {
        duration: 2500,
      });

      this.startAdd();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Save failed';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
    }
  }

  private validateDraft(d: BucketDraft): string | null {
    const required: Array<keyof BucketDraft> = [
      'bucket_id',
      'endpoint',
      'access_key_id',
      'secret_access_key',
      'bucket_name',
      'location',
    ];

    for (const k of required) {
      if (String(d[k] ?? '').trim().length === 0) {
        return `Missing required field: ${k}`;
      }
    }

    return null;
  }
}
