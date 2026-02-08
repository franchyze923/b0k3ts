import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

type KubeconfigListResponse =
  | string[]
  | { name: string }[]
  | { items: string[] }
  | { items: { name: string }[] };

@Injectable({ providedIn: 'root' })
export class KubernetesKubeconfigsService {
  private readonly apiBase = ''; // keep '' for same-origin; set if needed

  constructor(private readonly http: HttpClient) {}

  async uploadKubeconfig(name: string, file: File): Promise<void> {
    const safeName = encodeURIComponent(name);
    const url = `${this.apiBase}/api/v1/kubernetes/kubeconfigs/${safeName}`;

    const body = new FormData();
    body.append('file', file, file.name);

    await firstValueFrom(this.http.post<void>(url, body));
  }

  async listKubeconfigs(): Promise<string[]> {
    const url = `${this.apiBase}/api/v1/kubernetes/kubeconfigs`;
    const res = await firstValueFrom(this.http.get<KubeconfigListResponse>(url));

    const normalize = (v: unknown): string[] => {
      if (Array.isArray(v)) {
        return v
          .map((x) => (typeof x === 'string' ? x : (x as { name?: unknown })?.name))
          .filter((x): x is string => typeof x === 'string' && x.trim().length > 0)
          .sort((a, b) => a.localeCompare(b));
      }

      if (v && typeof v === 'object' && 'items' in v) {
        return normalize((v as { items?: unknown }).items);
      }

      return [];
    };

    return normalize(res);
  }

  async deleteKubeconfig(name: string): Promise<void> {
    const safeName = encodeURIComponent(name);
    const url = `${this.apiBase}/api/v1/kubernetes/kubeconfigs/${safeName}`;
    await firstValueFrom(this.http.delete<void>(url));
  }
}
