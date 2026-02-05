import { Component, computed, inject, signal } from '@angular/core';
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

import { GlobalService } from '../../services/global';
import { BucketConfig, BucketConfigsService } from '../../services/bucket-configs';

type BucketDraft = BucketConfig;

@Component({
  selector: 'app-settings',
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
  ],
  templateUrl: './settings.html',
  styleUrl: './settings.scss',
})
export class Settings {
  private readonly global = inject(GlobalService);
  private readonly bucketsService = inject(BucketConfigsService);
  private readonly snack = inject(MatSnackBar);

  readonly buckets = signal<BucketConfig[]>(this.bucketsService.list());

  readonly editingBucketId = signal<string | null>(null);

  readonly draft = signal<BucketDraft>({
    bucket_id: '',
    endpoint: '',
    access_key_id: '',
    secret_access_key: '',
    secure: true,
    bucket_name: '',
    location: '',
  });

  readonly isEditing = computed(() => this.editingBucketId() !== null);

  readonly displayedColumns = ['bucket_id', 'endpoint', 'bucket_name', 'location', 'secure', 'actions'] as const;

  constructor() {
    this.global.updateTitle('Settings');
  }

  startAdd(): void {
    this.editingBucketId.set(null);
    this.draft.set({
      bucket_id: '',
      endpoint: '',
      access_key_id: '',
      secret_access_key: '',
      secure: true,
      bucket_name: '',
      location: '',
    });
  }

  startEdit(row: BucketConfig): void {
    this.editingBucketId.set(row.bucket_id);
    this.draft.set({ ...row });
  }

  cancel(): void {
    this.startAdd();
  }

  delete(bucketId: string): void {
    this.buckets.set(this.bucketsService.remove(bucketId));
    this.snack.open('Bucket config deleted', 'Dismiss', { duration: 2500 });
    if (this.editingBucketId() === bucketId) this.startAdd();
  }

  save(): void {
    const d = this.draft();
    const validationError = this.validateDraft(d);
    if (validationError) {
      this.snack.open(validationError, 'Dismiss', { duration: 3500 });
      return;
    }

    try {
      const editingId = this.editingBucketId();
      if (editingId) {
        this.buckets.set(this.bucketsService.update(editingId, d));
        this.snack.open('Bucket config updated', 'Dismiss', { duration: 2500 });
      } else {
        this.buckets.set(this.bucketsService.add(d));
        this.snack.open('Bucket config added', 'Dismiss', { duration: 2500 });
      }
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

