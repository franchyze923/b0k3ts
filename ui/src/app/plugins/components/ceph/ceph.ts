import { Component, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';

import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import {CephBucketCredentials, CephBucketsService} from '../../services/cephbuckets';
import {JsonPipe} from '@angular/common';

@Component({
  selector: 'app-ceph',
  imports: [
    FormsModule,
    MatButtonModule,
    MatCardModule,
    MatFormFieldModule,
    MatInputModule,
    MatIconModule,
    MatProgressBarModule,
    MatSnackBarModule,
    JsonPipe,
  ],
  templateUrl: './ceph.html',
  styleUrl: './ceph.scss',
})
export class Ceph {
  readonly bucketName = signal<string>('');
  readonly loading = signal<boolean>(false);

  readonly buckets = signal<string[]>([]);
  readonly lastCreated = signal<CephBucketCredentials | null>(null);

  readonly canSubmit = computed(() => this.bucketName().trim().length > 0 && !this.loading());

  constructor(
    private readonly ceph: CephBucketsService,
    private readonly snack: MatSnackBar,
  ) {
    void this.refresh();
  }

  async refresh(): Promise<void> {
    this.loading.set(true);
    try {
      const list = await this.ceph.listBuckets();
      this.buckets.set(list);
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to load buckets';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
      this.buckets.set([]);
    } finally {
      this.loading.set(false);
    }
  }

  async create(): Promise<void> {
    const name = this.bucketName().trim();
    if (!name) return;

    this.loading.set(true);
    this.lastCreated.set(null);

    try {
      const creds = await this.ceph.createBucket(name);
      this.lastCreated.set({ ...creds, bucket_name: creds.bucket_name ?? name });
      this.snack.open(`Bucket "${name}" created`, 'Dismiss', { duration: 3000 });
      this.bucketName.set('');
      await this.refresh();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to create bucket';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
    } finally {
      this.loading.set(false);
    }
  }

  async delete(bucket: string): Promise<void> {
    if (!bucket) return;

    this.loading.set(true);
    try {
      await this.ceph.deleteBucket(bucket);
      this.snack.open(`Bucket "${bucket}" deleted`, 'Dismiss', { duration: 3000 });
      await this.refresh();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to delete bucket';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
    } finally {
      this.loading.set(false);
    }
  }

  trackByBucket = (_: number, b: string) => b;
}
