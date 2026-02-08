import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type CephBucketCredentials = {
  bucket_name?: string;
  access_key_id?: string;
  secret_access_key?: string;
  endpoint?: string;
  secure?: boolean;
  region?: string;
  location?: string;
};

export type KubernetesCommMode = 'incluster' | 'kubeconfig';

export type ListBucketsOptions = {
  namespace: string;
  mode: KubernetesCommMode;
  kubeconfigName?: string;
};

export type CephBucketRef = {
  bucket_name: string;
  obc: string;
};

@Injectable({ providedIn: 'root' })
export class CephBucketsService {
  private readonly apiBase = ''; // keep '' for same-origin; proxy handles /api/v1

  constructor(private readonly http: HttpClient) {}

  async listBuckets(opts: ListBucketsOptions): Promise<CephBucketRef[]> {
    const ns = encodeURIComponent(opts.namespace.trim());
    const params = new URLSearchParams();

    params.set('mode', opts.mode);

    if (opts.mode === 'kubeconfig') {
      const cfg = (opts.kubeconfigName ?? '').trim();
      if (cfg) params.set('config', cfg);
    }

    const url = `${this.apiBase}/api/v1/kubernetes/obc/${ns}?${params.toString()}`;

    const res = await firstValueFrom(this.http.get<unknown>(url));

    // New shape: [{ bucket_name, obc }]
    if (Array.isArray(res)) {
      return res
        .filter((x): x is Record<string, unknown> => !!x && typeof x === 'object')
        .map((x) => ({
          bucket_name: typeof x['bucket_name'] === 'string' ? x['bucket_name'] : '',
          obc: typeof x['obc'] === 'string' ? x['obc'] : '',
        }))
        .filter((x) => x.bucket_name.length > 0);
    }

    return [];
  }

  async getBucketCredentials(opts: ListBucketsOptions, obc: string): Promise<CephBucketCredentials> {
    const ns = encodeURIComponent(opts.namespace.trim());
    const obcName = encodeURIComponent(obc.trim());

    const params = new URLSearchParams();
    params.set('mode', opts.mode);

    if (opts.mode === 'kubeconfig') {
      const cfg = (opts.kubeconfigName ?? '').trim();
      if (cfg) params.set('config', cfg);
    }

    const url = `${this.apiBase}/api/v1/kubernetes/obc/${ns}/${obcName}/secret?${params.toString()}`;
    return await firstValueFrom(this.http.get<CephBucketCredentials>(url));
  }

  async applyObjectBucketClaim(opts: ListBucketsOptions, yaml: string): Promise<unknown> {
    const ns = encodeURIComponent(opts.namespace.trim());
    const params = new URLSearchParams();

    params.set('mode', opts.mode);

    if (opts.mode === 'kubeconfig') {
      const cfg = (opts.kubeconfigName ?? '').trim();
      if (cfg) params.set('config', cfg);
    }

    const url = `${this.apiBase}/api/v1/kubernetes/obc/${ns}/apply?${params.toString()}`;

    // Send raw YAML (common for “apply”-style endpoints)
    const headers = new HttpHeaders({
      'Content-Type': 'application/yaml',
      Accept: 'application/json',
    });

    return await firstValueFrom(
      this.http.post(url, yaml, {
        headers,
        responseType: 'json',
      }),
    );
  }

  async deleteObjectBucketClaim(opts: ListBucketsOptions, obc: string): Promise<void> {
    const ns = encodeURIComponent(opts.namespace.trim());
    const obcName = encodeURIComponent(obc.trim());

    const params = new URLSearchParams();
    params.set('mode', opts.mode);

    if (opts.mode === 'kubeconfig') {
      const cfg = (opts.kubeconfigName ?? '').trim();
      if (cfg) params.set('config', cfg);
    }

    const url = `${this.apiBase}/api/v1/kubernetes/obc/${ns}/${obcName}?${params.toString()}`;

    await firstValueFrom(this.http.delete<void>(url));
  }

}
