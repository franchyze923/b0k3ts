import { Component, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelectModule } from '@angular/material/select';
import { MatTableModule } from '@angular/material/table';
import { MatMenuModule } from '@angular/material/menu';
import { MatChipsModule } from '@angular/material/chips';
import { MatDialog, MatDialogModule } from '@angular/material/dialog';

import { ObjectStorageService } from '../../services/object-storage';
import {MovePrefixDialog, MovePrefixDialogResult} from '../move-prefix-dialog/move-prefix-dialog';

type BucketObject = {
  key: string;
  sizeBytes: number;
  lastModifiedIso: string;
};

@Component({
  selector: 'app-object-manager',
  imports: [
    CommonModule,
    FormsModule,
    MatCardModule,
    MatButtonModule,
    MatIconModule,
    MatFormFieldModule,
    MatSelectModule,
    MatTableModule,
    MatMenuModule,
    MatChipsModule,
    MatDialogModule,
  ],
  templateUrl: './object-manager.html',
  styleUrl: './object-manager.scss',
})
export class ObjectManager {
  private readonly dialog = inject(MatDialog);
  private readonly storage = inject(ObjectStorageService);

  // Starter data (replace with API integration)
  readonly buckets = signal<string[]>(['demo-bucket', 'logs', 'uploads']);
  readonly selectedBucket = signal<string>('demo-bucket');

  readonly objects = signal<BucketObject[]>([
    { key: 'reports/2026-02.pdf', sizeBytes: 182_220, lastModifiedIso: '2026-02-01T10:22:00Z' },
    { key: 'images/logo.png', sizeBytes: 12_442, lastModifiedIso: '2026-01-15T09:10:12Z' },
  ]);

  readonly displayedColumns = ['key', 'size', 'lastModified', 'actions'] as const;

  readonly objectCount = computed(() => this.objects().length);

  refresh(): void {
    // TODO: call your bucket service: listObjects(selectedBucket)
    // For now: no-op
  }

  onUploadFiles(files: FileList | null): void {
    if (!files || files.length === 0) return;

    // TODO: call your bucket service: uploadObject(bucket, file)
    const names = Array.from(files).map((f) => f.name);
    console.log('Upload to', this.selectedBucket(), names);
  }

  deleteObject(obj: BucketObject): void {
    // TODO: call your bucket service: deleteObject(bucket, obj.key)
    this.objects.update((list) => list.filter((o) => o.key !== obj.key));
  }

  private getPrefixFromKey(key: string): string {
    const parts = key.split('/');
    if (parts.length <= 1) return '';
    return parts.slice(0, -1).join('/') + '/';
  }

  async moveObject(obj: BucketObject): Promise<void> {
    const currentPrefix = this.getPrefixFromKey(obj.key);

    const ref = this.dialog.open(MovePrefixDialog, {
      width: '520px',
      data: { sourceKey: obj.key, currentPrefix },
    });

    const result = (await ref.afterClosed().toPromise()) as MovePrefixDialogResult;

    if (!result) return;

    const destinationPrefix = result.destinationPrefix ?? '';
    const bucket = this.selectedBucket();

    const { destinationKey } = await this.storage.moveObjectByPrefix({
      bucket,
      sourceKey: obj.key,
      destinationPrefix,
    });

    this.objects.update((list) =>
      list.map((o) => (o.key === obj.key ? { ...o, key: destinationKey } : o)),
    );
  }

  formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    const kb = bytes / 1024;
    if (kb < 1024) return `${kb.toFixed(1)} KB`;
    const mb = kb / 1024;
    return `${mb.toFixed(1)} MB`;
  }
}
