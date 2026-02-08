import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

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
}
