import { Component, computed, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';

import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { MatSelectModule } from '@angular/material/select';
import { MatTableModule } from '@angular/material/table';
import { MatDialog, MatDialogModule } from '@angular/material/dialog';
import { CephBucketCredentials, CephBucketsService, CephBucketRef, KubernetesCommMode } from '../../services/cephbuckets';
import { JsonPipe } from '@angular/common';
import { CephCredsDialogComponent } from '../cephcredssnack/cephcredssnack';
import { KubernetesKubeconfigsService } from '../../../core/services/kuberneteskubeconfigs';

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
    MatSelectModule,
    MatTableModule,
    MatDialogModule,
    JsonPipe,
  ],
  templateUrl: './ceph.html',
  styleUrl: './ceph.scss',
})
export class Ceph {
  // OBC (Kubernetes) fields
  readonly obcName = signal<string>(''); // metadata.name
  readonly bucketName = signal<string>(''); // spec.bucketName
  readonly objectBucketName = signal<string>(''); // spec.objectBucketName (optional; auto-filled if empty)
  readonly storageClassName = signal<string>(''); // spec.storageClassName
  readonly bucketProvisionerLabel = signal<string>('rook-ceph.ceph.rook.io-bucket'); // metadata.labels.bucket-provisioner

  readonly loading = signal<boolean>(false);

  readonly buckets = signal<CephBucketRef[]>([]);
  readonly lastCreated = signal<CephBucketCredentials | null>(null);

  readonly namespace = signal<string>('rook-ceph');
  readonly commMode = signal<KubernetesCommMode>('kubeconfig');
  readonly kubeconfigName = signal<string>('');

  readonly kubeconfigNames = signal<string[]>([]);
  readonly kubeconfigsLoading = signal<boolean>(false);

  private kubeconfigsLoadPromise: Promise<void> | null = null;

  readonly displayedColumns: ReadonlyArray<'obc' | 'bucket_name' | 'actions'> = ['obc', 'bucket_name', 'actions'];

  readonly needsKubeconfigName = computed(() => this.commMode() === 'kubeconfig');

  readonly canSubmit = computed(() => {
    const nsOk = this.namespace().trim().length > 0;
    const obcOk = this.obcName().trim().length > 0;
    const bucketOk = this.bucketName().trim().length > 0;
    const scOk = this.storageClassName().trim().length > 0;
    const cfgOk = this.commMode() !== 'kubeconfig' || this.kubeconfigName().trim().length > 0;
    return nsOk && obcOk && bucketOk && scOk && cfgOk && !this.loading();
  });

  constructor(
    private readonly ceph: CephBucketsService,
    private readonly snack: MatSnackBar,
    private readonly dialog: MatDialog,
    private readonly kubeconfigs: KubernetesKubeconfigsService,
  ) {
    void this.init();
  }

  private async init(): Promise<void> {
    // Ensure kubeconfig list is loaded (and default selected) BEFORE first buckets query
    if (this.commMode() === 'kubeconfig') {
      await this.refreshKubeconfigs();
    }
    await this.refresh();
  }

  async refreshKubeconfigs(): Promise<void> {
    if (this.kubeconfigsLoadPromise) return await this.kubeconfigsLoadPromise;

    this.kubeconfigsLoadPromise = (async () => {
      try {
        this.kubeconfigsLoading.set(true);
        const names = await this.kubeconfigs.listKubeconfigs();
        this.kubeconfigNames.set(names);

        // Per request: if list has 2+ items, choose the first as default.
        // (Also do the same for a single item so the value is never empty.)
        if (names.length > 0) {
          this.kubeconfigName.set(names[0]);
        }
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'Failed to load kubeconfigs';
        this.snack.open(msg, 'Dismiss', { duration: 4000 });
        this.kubeconfigNames.set([]);
        // keep kubeconfigName() as-is (user might type manually)
      } finally {
        this.kubeconfigsLoading.set(false);
        this.kubeconfigsLoadPromise = null;
      }
    })();

    return await this.kubeconfigsLoadPromise;
  }

  private buildObcYaml(): string {
    const ns = this.namespace().trim();
    const obc = this.obcName().trim();
    const bucket = this.bucketName().trim();

    const objectBucket =
      this.objectBucketName().trim().length > 0 ? this.objectBucketName().trim() : `obc-${ns}-${obc}`;

    const sc = this.storageClassName().trim();
    const prov = this.bucketProvisionerLabel().trim();

    // Keep it explicit and backend-friendly (no fancy YAML features)
    return [
      `apiVersion: objectbucket.io/v1alpha1`,
      `kind: ObjectBucketClaim`,
      `metadata:`,
      `  labels:`,
      `    bucket-provisioner: ${prov}`,
      `  name: ${obc}`,
      `  namespace: ${ns}`,
      `spec:`,
      `  bucketName: ${bucket}`,
      `  objectBucketName: ${objectBucket}`,
      `  storageClassName: ${sc}`,
      ``,
    ].join('\n');
  }

  async refresh(): Promise<void> {
    const ns = this.namespace().trim();
    if (!ns) {
      this.snack.open('Namespace is required', 'Dismiss', { duration: 3000 });
      this.buckets.set([]);
      return;
    }

    // Wait for kubeconfigs to load before hitting buckets endpoint (kubeconfig mode).
    if (this.commMode() === 'kubeconfig') {
      await this.refreshKubeconfigs();
    }

    this.loading.set(true);
    try {
      const list = await this.ceph.listBuckets({
        namespace: ns,
        mode: this.commMode(),
        kubeconfigName: this.kubeconfigName(),
      });
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
    if (!this.canSubmit()) return;

    const ns = this.namespace().trim();
    const yaml = this.buildObcYaml();

    this.loading.set(true);
    this.lastCreated.set(null);

    try {
      await this.ceph.applyObjectBucketClaim(
        {
          namespace: ns,
          mode: this.commMode(),
          kubeconfigName: this.kubeconfigName(),
        },
        yaml,
      );

      this.snack.open(`OBC "${this.obcName().trim()}" applied in namespace "${ns}"`, 'Dismiss', { duration: 3500 });

      await this.refresh();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to apply ObjectBucketClaim';
      this.snack.open(msg, 'Dismiss', { duration: 4500 });
    } finally {
      this.loading.set(false);
    }
  }

  async getCredentials(row: CephBucketRef): Promise<void> {
    const ns = this.namespace().trim();
    if (!ns) {
      this.snack.open('Namespace is required', 'Dismiss', { duration: 3000 });
      return;
    }
    if (!row?.obc?.trim()) {
      this.snack.open('Missing OBC name', 'Dismiss', { duration: 3000 });
      return;
    }

    this.loading.set(true);
    try {
      const creds = await this.ceph.getBucketCredentials(
        {
          namespace: ns,
          mode: this.commMode(),
          kubeconfigName: this.kubeconfigName(),
        },
        row.obc,
      );

      const ref = this.dialog.open(CephCredsDialogComponent, {
        data: {
          title: `Credentials for OBC "${row.obc}"`,
          credentials: creds as { AWS_ACCESS_KEY_ID?: string; AWS_SECRET_ACCESS_KEY?: string },
        },
        autoFocus: false,
        restoreFocus: true,
        hasBackdrop: true,
        panelClass: 'ceph-creds-dialog-panel',
      });

      window.setTimeout(() => ref.close(), 20000);
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to get credentials';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
    } finally {
      this.loading.set(false);
    }
  }

  async delete(bucket: CephBucketRef): Promise<void> {
    const ns = this.namespace().trim();
    if (!ns) {
      this.snack.open('Namespace is required', 'Dismiss', { duration: 3000 });
      return;
    }
    if (!bucket?.obc?.trim()) {
      this.snack.open('Missing OBC name', 'Dismiss', { duration: 3000 });
      return;
    }

    this.loading.set(true);
    try {
      await this.ceph.deleteObjectBucketClaim(
        {
          namespace: ns,
          mode: this.commMode(),
          kubeconfigName: this.kubeconfigName(),
        },
        bucket.obc,
      );

      this.snack.open(`OBC "${bucket.obc}" deleted`, 'Dismiss', { duration: 3000 });
      await this.refresh();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to delete OBC';
      this.snack.open(msg, 'Dismiss', { duration: 4000 });
    } finally {
      this.loading.set(false);
    }
  }

  trackByBucket = (_: number, b: CephBucketRef) => `${b.bucket_name}::${b.obc}`;
}
