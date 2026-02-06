import { Component, computed, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';

import { GlobalService } from '../../../services/global';
import { OidcConfig, OidcConfigService } from '../../../services/oidc-config';

@Component({
  selector: 'app-oidc-settings',
  imports: [
    CommonModule,
    FormsModule,
    MatCardModule,
    MatButtonModule,
    MatIconModule,
    MatFormFieldModule,
    MatInputModule,
    MatSnackBarModule,
  ],
  templateUrl: './oidc-settings.html',
  styleUrl: './oidc-settings.scss',
})
export class OidcSettings implements OnInit {
  private readonly global = inject(GlobalService);
  private readonly oidc = inject(OidcConfigService);
  private readonly snack = inject(MatSnackBar);

  readonly loading = signal(false);
  readonly saving = signal(false);

  readonly draft = signal<OidcConfig>({
    clientId: '',
    clientSecret: '',
    failRedirectUrl: '',
    passRedirectUrl: '',
    providerUrl: '',
    timeout: 0,
    jwtSecret: '',
    redirectUrl: '',
  });

  readonly busy = computed(() => this.loading() || this.saving());
  readonly canSave = computed(() => !this.busy());

  constructor() {
    this.global.updateTitle('Settings · OIDC');
  }

  async ngOnInit(): Promise<void> {
    await this.refresh();
  }

  async refresh(): Promise<void> {
    try {
      this.loading.set(true);
      const cfg = await this.oidc.getConfig();

      // Merge into defaults so template bindings always have defined keys
      this.draft.set({
        clientId: cfg.clientId ?? '',
        clientSecret: cfg.clientSecret ?? '',
        failRedirectUrl: cfg.failRedirectUrl ?? '',
        passRedirectUrl: cfg.passRedirectUrl ?? '',
        providerUrl: cfg.providerUrl ?? '',
        timeout: typeof cfg.timeout === 'number' ? cfg.timeout : 0,
        jwtSecret: cfg.jwtSecret ?? '',
        redirectUrl: cfg.redirectUrl ?? '',
      });
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to load OIDC config';
      this.snack.open(msg, 'Dismiss', { duration: 5000 });
    } finally {
      this.loading.set(false);
    }
  }

  async save(): Promise<void> {
    const d = this.draft();
    const err = this.validate(d);
    if (err) {
      this.snack.open(err, 'Dismiss', { duration: 4000 });
      return;
    }

    try {
      this.saving.set(true);
      await this.oidc.configure({
        ...d,
        timeout: d.timeout ? Number(d.timeout) : 0,
      });
      this.snack.open('OIDC configured', 'Dismiss', { duration: 2500 });

      // Reload from backend to reflect canonical values (and any server-side normalization)
      await this.refresh();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'OIDC configure failed';
      this.snack.open(msg, 'Dismiss', { duration: 5000 });
    } finally {
      this.saving.set(false);
    }
  }

  private validate(d: OidcConfig): string | null {
    const required: Array<keyof OidcConfig> = ['providerUrl', 'clientId', 'clientSecret', 'redirectUrl'];

    for (const k of required) {
      if (String(d[k] ?? '').trim().length === 0) return `Missing required field: ${String(k)}`;
    }

    const t = Number(d.timeout ?? 0);
    if (!Number.isFinite(t) || t < 0) return 'Timeout must be a non-negative number';
    return null;
  }
}
