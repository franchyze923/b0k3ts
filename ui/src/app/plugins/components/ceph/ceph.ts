import { Component, computed, OnInit, signal } from '@angular/core';
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
import {
  CephBucketCredentials,
  CephBucketsService,
  CephBucketRef,
  KubernetesCommMode,
} from '../../services/cephbuckets';
import { JsonPipe } from '@angular/common';
import { CephCredsDialogComponent } from '../cephcredssnack/cephcredssnack';
import { KubernetesKubeconfigsService } from '../../../core/services/kuberneteskubeconfigs';
import { BucketConfigsService } from '../../../core/services/bucket-configs';
import { Auth } from '../../../core/services/auth';
import { firstValueFrom } from 'rxjs';
import { SecureChoiceDialogComponent } from '../../../core/components/settings/securechoicedialog/securechoicedialog';

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
export class Ceph implements OnInit {
  readonly obcName = signal<string>('');
  readonly bucketName = signal<string>('');
  readonly objectBucketName = signal<string>('');
  readonly storageClassName = signal<string>('');
  readonly bucketProvisionerLabel = signal<string>('rook-ceph.ceph.rook.io-bucket');

  readonly loading = signal<boolean>(false);

  readonly buckets = signal<CephBucketRef[]>([]);
  readonly lastCreated = signal<CephBucketCredentials | null>(null);

  readonly namespace = signal<string>('rook-ceph');
  readonly commMode = signal<KubernetesCommMode>('kubeconfig');
  readonly kubeconfigName = signal<string>('');

  readonly kubeconfigNames = signal<string[]>([]);
  readonly kubeconfigsLoading = signal<boolean>(false);

  private kubeconfigsLoadPromise: Promise<void> | null = null;

  readonly displayedColumns: ReadonlyArray<'obc' | 'bucket_name' | 'actions'> = [
    'obc',
    'bucket_name',
    'actions',
  ];

  readonly needsKubeconfigName = computed(() => this.commMode() === 'kubeconfig');

  readonly canSubmit = computed(() => {
    const nsOk = this.namespace().trim().length > 0;
    const obcOk = this.obcName().trim().length > 0;
    const bucketOk = this.bucketName().trim().length > 0;
    const scOk = this.storageClassName().trim().length > 0;
    const cfgOk = this.commMode() !== 'kubeconfig' || this.kubeconfigName().trim().length > 0;
    return nsOk && obcOk && bucketOk && scOk && cfgOk && !this.loading();
  });

  readonly connectedBucketIds = signal<Set<string>>(new Set());
  private readonly currentUserEmail = signal<string>('');

  constructor(
    private readonly ceph: CephBucketsService,
    private readonly snack: MatSnackBar,
    private readonly dialog: MatDialog,
    private readonly kubeconfigs: KubernetesKubeconfigsService,
    private readonly bucketConfigs: BucketConfigsService,
    private readonly auth: Auth,
  ) {}

  ngOnInit() {
    void this.init();
  }

  private async init(): Promise<void> {
    if (this.commMode() === 'kubeconfig') {
      await this.refreshKubeconfigs();
    }
    await this.refreshConnectedBuckets();
    await this.refresh();
  }

  private async refreshConnectedBuckets(): Promise<void> {
    try {
      const list = await this.bucketConfigs.listConnections();
      const ids = new Set<string>();
      for (const c of list ?? []) {
        if (typeof c?.bucket_id === 'string' && c.bucket_id.trim()) ids.add(c.bucket_id.trim());
      }
      this.connectedBucketIds.set(ids);
    } catch {
      this.connectedBucketIds.set(new Set());
    }
  }

  private async ensureCurrentUserEmailLoaded(): Promise<string> {
    const cached = this.currentUserEmail().trim();
    if (cached) return cached;

    const token = this.auth.getToken();
    if (!token) return '';

    try {
      const res = await this.auth.authenticateAny(token);
      const email = res.user_info?.email?.trim() ?? '';
      if (email) this.currentUserEmail.set(email);
      return email;
    } catch {
      return '';
    }
  }

  isConnected(row: CephBucketRef): boolean {
    const id = String(row?.obc ?? '').trim();
    if (!id) return false;
    return this.connectedBucketIds().has(id);
  }

  private promptForEndpoint(bucketLabel: string): string | null {
    const raw = globalThis.prompt(
      `Endpoint is missing for "${bucketLabel}".\n\nEnter S3 endpoint (example: s3.example.com:80):`,
      '',
    );
    const v = (raw ?? '').trim();
    return v.length > 0 ? v : null;
  }

  private async promptForSecure(
    bucketLabel: string,
    defaultSecure: boolean,
  ): Promise<boolean | undefined> {
    const ref = this.dialog.open(SecureChoiceDialogComponent, {
      width: '420px',
      data: { bucketLabel, defaultSecure },
      autoFocus: false,
      restoreFocus: true,
      hasBackdrop: true,
    });

    return (await firstValueFrom(ref.afterClosed())) as boolean | undefined;
  }

  private inferSecureFromEndpoint(endpoint: string): boolean {
    const e = endpoint.trim().toLowerCase();
    if (e.startsWith('https://')) return true;
    if (e.startsWith('http://')) return false;
    // If scheme-less, safest default is secure
    return true;
  }

  private ensureNamespaceOrNotify(): string | null {
    const ns = this.namespace().trim();
    if (!ns) {
      this.snack.open('Namespace is required', 'Dismiss', { duration: 3000 });
      return null;
    }
    return ns;
  }

  private ensureObcOrNotify(row: CephBucketRef): string | null {
    const obc = String(row?.obc ?? '').trim();
    if (!obc) {
      this.snack.open('Missing OBC name', 'Dismiss', { duration: 3000 });
      return null;
    }
    return obc;
  }

  private ensureNotAlreadyConnectedOrNotify(row: CephBucketRef): boolean {
    if (this.isConnected(row)) {
      this.snack.open('Bucket is already connected', 'Dismiss', { duration: 2500 });
      return false;
    }
    return true;
  }

  private async fetchBucketCreds(
    ns: string,
    obc: string,
  ): Promise<CephBucketCredentials & Record<string, unknown>> {
    return (await this.ceph.getBucketCredentials(
      {
        namespace: ns,
        mode: this.commMode(),
        kubeconfigName: this.kubeconfigName(),
      },
      obc,
    )) as CephBucketCredentials & Record<string, unknown>;
  }

  private resolveEndpointFromCreds(creds: CephBucketCredentials & Record<string, unknown>): string {
    return String((creds as any)?.endpoint ?? '').trim();
  }

  private async maybePromptEndpointOverride(args: {
    row: CephBucketRef;
    endpointFromCreds: string;
  }): Promise<string | undefined> {
    const { row, endpointFromCreds } = args;

    // Ask for endpoint ONLY if missing
    if (endpointFromCreds) return undefined;

    const provided = this.promptForEndpoint(String(row.bucket_name || row.obc || 'bucket'));
    if (!provided) {
      this.snack.open('Endpoint is required to connect this bucket', 'Dismiss', { duration: 3500 });
      return undefined; // caller should treat as "cancelled"
    }
    return provided;
  }

  private async maybePromptSecureOverride(args: {
    row: CephBucketRef;
    creds: CephBucketCredentials & Record<string, unknown>;
    endpointToUseForDefault: string;
  }): Promise<boolean | undefined> {
    const { row, creds, endpointToUseForDefault } = args;

    // Ask for secure if backend didn't provide it
    const secureFromCreds = (creds as any)?.secure;
    if (typeof secureFromCreds === 'boolean') return undefined;

    const defaultSecure = this.inferSecureFromEndpoint(endpointToUseForDefault.trim());
    const choice = await this.promptForSecure(
      String(row.bucket_name || row.obc || 'bucket'),
      defaultSecure,
    );

    // If user cancelled the dialog, don't silently proceed with an implicit choice
    if (typeof choice !== 'boolean') {
      this.snack.open('Cancelled: security choice is required', 'Dismiss', { duration: 3000 });
      return undefined; // caller should treat as "cancelled"
    }

    return choice;
  }

  private getMissingBucketConfigFields(cfg: {
    bucket_id?: string;
    endpoint?: string;
    access_key_id?: string;
    secret_access_key?: string;
    bucket_name?: string;
    location?: string;
  }): string[] {
    const missing: string[] = [];
    if (!cfg.bucket_id) missing.push('bucket_id');
    if (!cfg.endpoint) missing.push('endpoint');
    if (!cfg.access_key_id) missing.push('access_key_id');
    if (!cfg.secret_access_key) missing.push('secret_access_key');
    if (!cfg.bucket_name) missing.push('bucket_name');
    if (!cfg.location) missing.push('location');
    return missing;
  }

  private notifyMissingConfigAndReturnFalse(missing: string[]): boolean {
    if (missing.length === 0) return true;

    this.snack.open(`Cannot connect bucket (missing: ${missing.join(', ')})`, 'Dismiss', {
      duration: 4500,
    });
    return false;
  }

  async connectBucket(row: CephBucketRef): Promise<void> {
    const ns = this.ensureNamespaceOrNotify();
    if (!ns) return;

    const obc = this.ensureObcOrNotify(row);
    if (!obc) return;

    if (!this.ensureNotAlreadyConnectedOrNotify(row)) return;

    this.loading.set(true);
    try {
      const creds = await this.fetchBucketCreds(ns, obc);
      const currentUserEmail = await this.ensureCurrentUserEmailLoaded();

      const endpointFromCreds = this.resolveEndpointFromCreds(creds);
      const endpointOverride = await this.maybePromptEndpointOverride({ row, endpointFromCreds });
      if (!endpointFromCreds && !endpointOverride) return; // endpoint prompt cancelled

      const endpointForDefault = (endpointOverride ?? endpointFromCreds).trim();
      const secureOverride = await this.maybePromptSecureOverride({
        row,
        creds,
        endpointToUseForDefault: endpointForDefault,
      });
      if (typeof (creds as any)?.secure !== 'boolean' && typeof secureOverride !== 'boolean')
        return; // secure prompt cancelled

      const cfg = this.cephCredsToBucketConfig({
        row,
        creds,
        currentUserEmail,
        endpointOverride,
        secureOverride,
      });

      const missing = this.getMissingBucketConfigFields(cfg);
      if (!this.notifyMissingConfigAndReturnFalse(missing)) return;

      if (!currentUserEmail) {
        this.snack.open(
          'Connected bucket will be created, but current user email could not be determined for allowed users.',
          'Dismiss',
          { duration: 5000 },
        );
      }

      await this.bucketConfigs.addConnection(cfg);
      await this.refreshConnectedBuckets();

      this.snack.open(`Connected "${cfg.bucket_name}" (id: ${cfg.bucket_id})`, 'Dismiss', {
        duration: 3000,
      });
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to connect bucket';
      this.snack.open(msg, 'Dismiss', { duration: 4500 });
    } finally {
      this.loading.set(false);
    }
  }

  private cephCredsToBucketConfig(args: {
    row: CephBucketRef;
    creds: CephBucketCredentials & Record<string, unknown>;
    currentUserEmail: string;
    endpointOverride?: string;
    secureOverride?: boolean;
  }) {
    const { row, creds, currentUserEmail, endpointOverride, secureOverride } = args;

    const bucket_id = String(row?.obc ?? '').trim();
    const bucket_name = String(row?.bucket_name ?? '').trim();

    const endpoint =
      String(endpointOverride ?? '').trim() || String((creds as any)?.endpoint ?? '').trim();

    const access_key_id = String(
      (creds as any)?.access_key_id ?? (creds as any)?.AWS_ACCESS_KEY_ID ?? '',
    ).trim();

    const secret_access_key = String(
      (creds as any)?.secret_access_key ?? (creds as any)?.AWS_SECRET_ACCESS_KEY ?? '',
    ).trim();

    const location =
      String((creds as any)?.location ?? (creds as any)?.region ?? '').trim() || 'us-east-1';

    const secureFromCreds = (creds as any)?.secure as unknown;
    let secure: boolean;
    if (typeof secureOverride === 'boolean') {
      secure = secureOverride;
    } else if (typeof secureFromCreds === 'boolean') {
      secure = secureFromCreds;
    } else {
      secure = this.inferSecureFromEndpoint(endpoint);
    }

    const authorized_users = currentUserEmail ? [currentUserEmail] : [];
    const authorized_groups: string[] = [];

    return {
      bucket_id,
      endpoint,
      access_key_id,
      secret_access_key,
      secure,
      bucket_name,
      location,
      authorized_users,
      authorized_groups,
    };
  }

  async refreshKubeconfigs(): Promise<void> {
    if (this.kubeconfigsLoadPromise) return await this.kubeconfigsLoadPromise;

    this.kubeconfigsLoadPromise = (async () => {
      try {
        this.kubeconfigsLoading.set(true);
        const names = await this.kubeconfigs.listKubeconfigs();
        this.kubeconfigNames.set(names);

        if (names.length > 0) {
          this.kubeconfigName.set(names[0]);
        }
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'Failed to load kubeconfigs';
        this.snack.open(msg, 'Dismiss', { duration: 4000 });
        this.kubeconfigNames.set([]);
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
      this.objectBucketName().trim().length > 0
        ? this.objectBucketName().trim()
        : `obc-${ns}-${obc}`;

    const sc = this.storageClassName().trim();
    const prov = this.bucketProvisionerLabel().trim();

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
      await this.refreshConnectedBuckets();
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

      this.snack.open(`OBC "${this.obcName().trim()}" applied in namespace "${ns}"`, 'Dismiss', {
        duration: 3500,
      });

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

      globalThis.setTimeout(() => ref.close(), 20000);
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

  _ = (_: number, b: CephBucketRef) => `${b.bucket_name}::${b.obc}`;
}
