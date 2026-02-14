import { Component, computed, effect, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelectModule } from '@angular/material/select';
import { MatMenuModule } from '@angular/material/menu';
import { MatChipsModule } from '@angular/material/chips';
import { MatDialog, MatDialogModule } from '@angular/material/dialog';
import { MatTreeModule, MatTreeNestedDataSource } from '@angular/material/tree';
import { MatInputModule } from '@angular/material/input';
import { MatTableModule } from '@angular/material/table';

import { ObjectStorageService } from '../../services/object-storage';
import { MovePrefixDialog, MovePrefixDialogResult } from '../move-prefix-dialog/move-prefix-dialog';
import { GlobalService } from '../../services/global';
import { BucketConfigsService } from '../../services/bucket-configs';
import { firstValueFrom } from 'rxjs';
import { MatProgressBar } from '@angular/material/progress-bar';

type BucketObject = {
  key: string;
  sizeBytes: number;
  contentType: string;
};

type TreeNode =
  | {
      kind: 'dir';
      name: string;
      path: string; // unique path for the directory (no trailing slash)
      children: TreeNode[];
    }
  | {
      kind: 'file';
      name: string;
      path: string; // full key
      obj: BucketObject;
    };

type ExplorerRow =
  | {
      kind: 'dir';
      name: string;
      path: string; // no trailing slash ('' means root)
      itemCount: number;
    }
  | {
      kind: 'file';
      name: string;
      path: string; // full key
      obj: BucketObject;
    };

type UploadTask = {
  id: string;
  fileName: string;
  key: string;
  status: 'queued' | 'uploading' | 'done' | 'error';
  percent: number; // 0..100
  uploadedBytes: number;
  totalBytes: number;
  partNumber?: number;
  partCount?: number;
  errorMessage?: string;
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
    MatMenuModule,
    MatChipsModule,
    MatDialogModule,
    MatTreeModule,
    MatInputModule,
    MatTableModule,
    MatProgressBar,
  ],
  templateUrl: './object-manager.html',
  styleUrl: './object-manager.scss',
})
export class ObjectManager implements OnInit {
  private readonly global = inject(GlobalService);
  private readonly dialog = inject(MatDialog);
  private readonly storage = inject(ObjectStorageService);
  private readonly bucketConfigs = inject(BucketConfigsService);

  constructor() {
    this.global.updateTitle('Object Manager');

    effect(() => {
      this.dataSource.data = this.buildTree(this.objects());
    });
  }

  ngOnInit() {
    void this.loadBucketsFromBackend();
  }

  readonly buckets = signal<string[]>([]);
  readonly selectedBucket = signal<string>('');

  readonly objects = signal<BucketObject[]>([]);
  readonly objectCount = computed(() => this.objects().length);

  readonly uploadPrefix = signal<string>(''); // e.g. "reports/2026/"

  // Explorer navigation state: current folder ('' is root, otherwise no trailing slash)
  readonly currentDir = signal<string>('');

  // “Details view” columns
  readonly displayedColumns: Array<'name' | 'size' | 'type' | 'actions'> = [
    'name',
    'size',
    'type',
    'actions',
  ];

  readonly uploadTasks = signal<UploadTask[]>([]);
  readonly isUploading = computed(() =>
    this.uploadTasks().some((t) => t.status === 'queued' || t.status === 'uploading'),
  );

  private updateUploadTask(id: string, patch: Partial<UploadTask>): void {
    this.uploadTasks.update((list) => list.map((t) => (t.id === id ? { ...t, ...patch } : t)));
  }

  clearCompletedUploads(): void {
    this.uploadTasks.update((list) => list.filter((t) => t.status !== 'done'));
  }

  clearAllUploads(): void {
    this.uploadTasks.set([]);
  }

  private normalizePrefix(prefix: string): string {
    const s = prefix.trim();
    if (s === '') return '';

    let start = 0;
    let end = s.length;

    while (start < end && (s.codePointAt(start) ?? -1) === 47) start++; // '/'
    while (end > start && (s.codePointAt(end - 1) ?? -1) === 47) end--; // '/'

    const core = s.slice(start, end);
    return core === '' ? '' : core + '/';
  }

  private normalizeDirPath(path: string): string {
    const s = path.trim();

    let start = 0;
    let end = s.length;

    while (start < end && (s.codePointAt(start) ?? -1) === 47) start++;
    while (end > start && (s.codePointAt(end - 1) ?? -1) === 47) end--;

    return s.slice(start, end);
  }

  private dirToPrefix(dir: string): string {
    const clean = this.normalizeDirPath(dir);
    return clean ? `${clean}/` : '';
  }

  readonly dataSource = new MatTreeNestedDataSource<TreeNode>();

  private buildTree(objects: BucketObject[]): TreeNode[] {
    const root: { kind: 'dir'; name: string; path: string; children: TreeNode[] } = {
      kind: 'dir',
      name: '',
      path: '',
      children: [],
    };

    const ensureDir = (children: TreeNode[], name: string, path: string) => {
      const isDirNamed = (c: TreeNode): c is Extract<TreeNode, { kind: 'dir'; name: string }> =>
        c.kind === 'dir' && c.name === name;

      let dir = children.find(isDirNamed);

      if (!dir) {
        dir = { kind: 'dir', name, path, children: [] };
        children.push(dir);
      }

      return dir;
    };

    for (const obj of objects) {
      const parts = obj.key.split('/').filter((p) => p.length > 0);
      if (parts.length === 0) continue;

      let current = root;
      let currentPath = '';

      for (let i = 0; i < parts.length; i++) {
        const part = parts[i];
        const isLast = i === parts.length - 1;

        currentPath = currentPath ? `${currentPath}/${part}` : part;

        if (!isLast) {
          current = ensureDir(current.children, part, currentPath);
          continue;
        }

        current.children.push({
          kind: 'file',
          name: part,
          path: obj.key,
          obj,
        });
      }
    }

    const sortRec = (nodes: TreeNode[]) => {
      nodes.sort((a, b) => {
        if (a.kind !== b.kind) return a.kind === 'dir' ? -1 : 1;
        return a.name.localeCompare(b.name);
      });
      for (const n of nodes) {
        if (n.kind === 'dir') sortRec(n.children);
      }
    };

    sortRec(root.children);
    return root.children;
  }

  // --- Explorer “address bar” (breadcrumbs) ---
  readonly breadcrumbs = computed(() => {
    const dir = this.normalizeDirPath(this.currentDir());
    if (!dir) return [] as Array<{ label: string; path: string }>;

    const parts = dir.split('/').filter(Boolean);
    const crumbs: Array<{ label: string; path: string }> = [];
    let acc = '';
    for (const p of parts) {
      acc = acc ? `${acc}/${p}` : p;
      crumbs.push({ label: p, path: acc });
    }
    return crumbs;
  });

  goToDir(path: string): void {
    this.currentDir.set(this.normalizeDirPath(path));
  }

  goUp(): void {
    const dir = this.normalizeDirPath(this.currentDir());
    if (!dir) return;
    const parts = dir.split('/').filter(Boolean);
    parts.pop();
    this.currentDir.set(parts.join('/'));
  }

  // --- Right pane rows (folders + files of currentDir) ---
  readonly explorerRows = computed<ExplorerRow[]>(() => {
    const prefix = this.dirToPrefix(this.currentDir());
    const list = this.objects();

    const folderNames = new Map<string, number>(); // name -> count of immediate children (rough)
    const files: ExplorerRow[] = [];

    for (const obj of list) {
      if (!obj.key.startsWith(prefix)) continue;

      const rest = obj.key.slice(prefix.length);
      if (!rest) continue;

      const parts = rest.split('/').filter(Boolean);
      if (parts.length === 0) continue;

      if (parts.length === 1) {
        files.push({
          kind: 'file',
          name: parts[0],
          path: obj.key,
          obj,
        });
      } else {
        const dirName = parts[0];
        folderNames.set(dirName, (folderNames.get(dirName) ?? 0) + 1);
      }
    }

    const dirs: ExplorerRow[] = Array.from(folderNames.entries()).map(([name, count]) => ({
      kind: 'dir',
      name,
      path: prefix ? `${prefix}${name}`.replace(/\/$/, '') : name,
      itemCount: count,
    }));

    dirs.sort((a, b) => a.name.localeCompare(b.name));
    files.sort((a, b) => a.name.localeCompare(b.name));

    return [...dirs, ...files];
  });

  onRowActivate(row: ExplorerRow): void {
    if (row.kind === 'dir') this.goToDir(row.path);
  }

  fileTypeLabel(row: ExplorerRow): string {
    if (row.kind === 'dir') return 'File folder';
    const name = row.name;
    const dot = name.lastIndexOf('.');
    if (dot > 0 && dot < name.length - 1) return `${name.slice(dot + 1).toUpperCase()} File`;
    return row.obj.contentType || 'File';
  }

  private async loadBucketsFromBackend(): Promise<void> {
    const connections = await this.bucketConfigs.listConnections();
    const bucketNames = Array.from(new Set(connections.map((c) => c.bucket_name))).sort((a, b) =>
      a.localeCompare(b),
    );

    this.buckets.set(bucketNames);

    const current = this.selectedBucket();

    let next = '';
    if (current && bucketNames.includes(current)) {
      next = current;
    } else if (bucketNames.length > 0) {
      next = bucketNames[0];
    }

    this.selectedBucket.set(next);

    this.currentDir.set('');

    if (next) {
      await this.refresh();
    } else {
      this.objects.set([]);
    }
  }

  async onBucketChange(bucket: string): Promise<void> {
    this.selectedBucket.set(bucket ?? '');
    this.currentDir.set('');
    await this.refresh();
  }

  async refresh(): Promise<void> {
    const bucket = this.selectedBucket();
    if (!bucket) {
      this.objects.set([]);
      return;
    }

    try {
      const items = await this.storage.listObjects({ bucket });
      this.objects.set(
        items.map((o) => ({
          key: o.key,
          sizeBytes: o.size,
          contentType: o.content_type,
        })),
      );
    } catch (e) {
      this.objects.set([]);
      const msg = e instanceof Error ? e.message : 'Failed to load objects';
      console.error(msg, e);
    }
  }

  async onUploadFiles(files: FileList | null): Promise<void> {
    if (!files || files.length === 0) return;

    const bucket = this.selectedBucket();
    if (!bucket) return;

    // Default upload location = current folder (Explorer-style)
    const currentPrefix = this.dirToPrefix(this.currentDir());
    const manualPrefix = this.normalizePrefix(this.uploadPrefix());
    const prefix = manualPrefix || currentPrefix;

    // Create tasks up-front so UI shows a queue immediately
    const tasks: UploadTask[] = Array.from(files).map((file) => {
      const key = `${prefix}${file.name}`;
      const id = `${Date.now()}-${crypto.randomUUID()}-${key}`;
      return {
        id,
        fileName: file.name,
        key,
        status: 'queued',
        percent: 0,
        uploadedBytes: 0,
        totalBytes: file.size,
      };
    });

    this.uploadTasks.update((list) => [...tasks, ...list]);

    // Upload sequentially (matches your current behavior; easier on presign + network)
    for (const file of Array.from(files)) {
      const key = `${prefix}${file.name}`;
      const task = this.uploadTasks().find((t) => t.key === key && t.totalBytes === file.size);
      const id = task?.id ?? `${Date.now()}-${crypto.randomUUID()}-${key}`;

      this.updateUploadTask(id, { status: 'uploading', errorMessage: undefined });

      try {
        await this.storage.uploadObjectMultipart({
          bucket,
          key,
          file,
          contentType: file.type || undefined,
          onProgress: ({ percent, uploadedBytes, totalBytes, partNumber, partCount }) => {
            this.updateUploadTask(id, {
              status: 'uploading',
              percent,
              uploadedBytes,
              totalBytes,
              partNumber,
              partCount,
            });
          },
        });

        this.updateUploadTask(id, { status: 'done', percent: 100, uploadedBytes: file.size });
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'Upload failed';
        this.updateUploadTask(id, { status: 'error', errorMessage: msg });
        console.error(msg, e);
      }
    }

    await this.refresh();
  }

  async deleteObject(obj: BucketObject): Promise<void> {
    const bucket = this.selectedBucket();
    if (!bucket) return;

    await this.storage.deleteObject({ bucket, filename: obj.key });
    await this.refresh();
  }

  private fileNameFromKey(key: string): string {
    // Avoid regex: trim trailing slashes with a linear scan
    let end = key.length;
    while (end > 0 && (key.codePointAt(end - 1) ?? -1) === 47) end--; // '/'

    const clean = key.slice(0, end);
    const parts = clean.split('/');
    return parts.at(-1) || 'download';
  }

  async downloadObject(obj: BucketObject): Promise<void> {
    const bucket = this.selectedBucket();
    if (!bucket) return;

    await this.storage.downloadObjectNative({
      bucket,
      key: obj.key,
      filename: this.fileNameFromKey(obj.key), // optional, but nice for Content-Disposition
      disposition: 'attachment',
      // openInNewTab: true, // optional
    });
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

    const result = (await firstValueFrom(ref.afterClosed())) as MovePrefixDialogResult;
    if (!result) return;

    const destinationPrefix = result.destinationPrefix ?? '';
    const bucket = this.selectedBucket();
    if (!bucket) return;

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
