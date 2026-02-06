import { Component, computed, effect, inject, signal } from '@angular/core';
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
import { MatTreeModule } from '@angular/material/tree';

import { FlatTreeControl } from '@angular/cdk/tree';
import { MatTreeFlatDataSource, MatTreeFlattener } from '@angular/material/tree';

import { ObjectStorageService } from '../../services/object-storage';
import { MovePrefixDialog, MovePrefixDialogResult } from '../move-prefix-dialog/move-prefix-dialog';
import { GlobalService } from '../../services/global';
import { BucketConfigsService } from '../../services/bucket-configs';

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

type FlatNode = {
  kind: 'dir' | 'file';
  name: string;
  path: string;
  level: number;
  expandable: boolean;
  obj?: BucketObject;
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
  ],
  templateUrl: './object-manager.html',
  styleUrl: './object-manager.scss',
})
export class ObjectManager {
  private readonly global = inject(GlobalService);
  private readonly dialog = inject(MatDialog);
  private readonly storage = inject(ObjectStorageService);
  private readonly bucketConfigs = inject(BucketConfigsService);

  constructor() {
    this.global.updateTitle('Object Manager');
    void this.loadBucketsFromBackend();

    // Keep the tree in sync with current objects()
    effect(() => {
      this.dataSource.data = this.buildTree(this.objects());
      // Optional: auto-expand root level
      // this.treeControl.expandAll();
    });
  }

  readonly buckets = signal<string[]>([]);
  readonly selectedBucket = signal<string>('');

  readonly objects = signal<BucketObject[]>([]);

  readonly objectCount = computed(() => this.objects().length);

  // ---- Tree setup (Flat tree) ----
  private readonly treeFlattener = new MatTreeFlattener<TreeNode, FlatNode>(
    (node: TreeNode, level: number): FlatNode => {
      if (node.kind === 'dir') {
        return {
          kind: 'dir',
          name: node.name,
          path: node.path,
          level,
          expandable: node.children.length > 0,
        };
      }
      return {
        kind: 'file',
        name: node.name,
        path: node.path,
        level,
        expandable: false,
        obj: node.obj,
      };
    },
    (flatNode) => flatNode.level,
    (flatNode) => flatNode.expandable,
    (node) => (node.kind === 'dir' ? node.children : []),
  );

  readonly treeControl = new FlatTreeControl<FlatNode>(
    (node) => node.level,
    (node) => node.expandable,
  );

  readonly dataSource = new MatTreeFlatDataSource(this.treeControl, this.treeFlattener);

  readonly hasChild = (_: number, node: FlatNode) => node.expandable;

  private buildTree(objects: BucketObject[]): TreeNode[] {
    // Root is an implicit folder; we return its children
    const root: { kind: 'dir'; name: string; path: string; children: TreeNode[] } = {
      kind: 'dir',
      name: '',
      path: '',
      children: [],
    };

    const ensureDir = (children: TreeNode[], name: string, path: string) => {
      let dir = children.find((c) => c.kind === 'dir' && c.name === name) as TreeNode | undefined;
      if (!dir) {
        dir = { kind: 'dir', name, path, children: [] };
        children.push(dir);
      }
      return dir as Extract<TreeNode, { kind: 'dir' }>;
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

        // Leaf file node
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
        if (a.kind !== b.kind) return a.kind === 'dir' ? -1 : 1; // dirs first
        return a.name.localeCompare(b.name);
      });
      for (const n of nodes) {
        if (n.kind === 'dir') sortRec(n.children);
      }
    };

    sortRec(root.children);
    return root.children;
  }

  // ---- Data loading / actions ----
  private async loadBucketsFromBackend(): Promise<void> {
    const connections = await this.bucketConfigs.listConnections();
    const bucketNames = Array.from(new Set(connections.map((c) => c.bucket_name))).sort();

    this.buckets.set(bucketNames);

    const current = this.selectedBucket();
    const next =
      current && bucketNames.includes(current) ? current : bucketNames.length > 0 ? bucketNames[0] : '';

    this.selectedBucket.set(next);

    if (next) {
      await this.refresh();
    } else {
      this.objects.set([]);
    }
  }

  async refresh(): Promise<void> {
    const bucket = this.selectedBucket();
    if (!bucket) {
      this.objects.set([]);
      return;
    }

    const items = await this.storage.listObjects({ bucket });
    this.objects.set(
      items.map((o) => ({
        key: o.key,
        sizeBytes: o.size,
        contentType: o.content_type,
      })),
    );
  }

  async onUploadFiles(files: FileList | null): Promise<void> {
    if (!files || files.length === 0) return;

    const bucket = this.selectedBucket();
    if (!bucket) return;

    for (const file of Array.from(files)) {
      const buffer = await file.arrayBuffer();
      const bytes = Array.from(new Uint8Array(buffer));

      await this.storage.uploadObject({
        bucket,
        key: file.name,
        bytes,
        contentType: file.type || undefined,
      });
    }

    await this.refresh();
  }

  async deleteObject(obj: BucketObject): Promise<void> {
    const bucket = this.selectedBucket();
    if (!bucket) return;

    await this.storage.deleteObject({ bucket, key: obj.key });
    await this.refresh();
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
