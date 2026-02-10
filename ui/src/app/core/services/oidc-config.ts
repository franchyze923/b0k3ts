import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type OidcConfig = {
  clientId?: string;
  clientSecret?: string;
  failRedirectUrl?: string;
  passRedirectUrl?: string;
  providerUrl?: string;
  timeout?: number;
  jwtSecret?: string;
  redirectUrl?: string;
  adminGroup?: string;
};

@Injectable({ providedIn: 'root' })
export class OidcConfigService {
  private readonly apiBase = '';

  constructor(private readonly http: HttpClient) {}

  async getConfig(): Promise<OidcConfig> {
    const url = `${this.apiBase}/api/v1/oidc/config`;
    return await firstValueFrom(this.http.get<OidcConfig>(url));
  }

  async configure(cfg: OidcConfig): Promise<void> {
    const url = `${this.apiBase}/api/v1/oidc/configure`;
    await firstValueFrom(this.http.post<void>(url, cfg));
  }
}
