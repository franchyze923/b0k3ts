import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';

import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatTableModule } from '@angular/material/table';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';

import { LocalUsersService, LocalUserSafe } from '../../../services/localuser';
import { firstValueFrom } from 'rxjs';

type Draft = {
  username: string;
  password: string;
  password2: string;
  administrator: boolean;
};

@Component({
  selector: 'app-users',
  imports: [
    FormsModule,
    MatCardModule,
    MatFormFieldModule,
    MatInputModule,
    MatButtonModule,
    MatIconModule,
    MatTableModule,
    MatProgressSpinnerModule,
    MatSlideToggleModule,
  ],
  templateUrl: './users.html',
  styleUrl: './users.scss',
})
export class Users {
  private readonly api = inject(LocalUsersService);

  readonly loading = signal(false);
  readonly working = signal(false);
  readonly error = signal<string | null>(null);

  readonly draft = signal<Draft>({
    username: '',
    password: '',
    password2: '',
    administrator: false,
  });

  readonly users = signal<LocalUserSafe[]>([]);
  readonly displayedColumns = ['username', 'administrator', 'disabled', 'actions'];

  readonly canLookup = computed(
    () => !!this.draft().username.trim() && !this.loading() && !this.working(),
  );
  readonly canCreate = computed(() => {
    const d = this.draft();
    return (
      !!d.username.trim() &&
      !!d.password &&
      d.password === d.password2 &&
      !this.loading() &&
      !this.working()
    );
  });

  readonly canResetPassword = computed(() => {
    const d = this.draft();
    return (
      !!d.username.trim() &&
      !!d.password &&
      d.password === d.password2 &&
      !this.loading() &&
      !this.working()
    );
  });

  private upsertUser(u: LocalUserSafe) {
    const list = this.users();
    const idx = list.findIndex((x) => x.username === u.username);
    if (idx >= 0) {
      const next = list.slice();
      next[idx] = { ...next[idx], ...u };
      this.users.set(next);
    } else {
      this.users.set([u, ...list]);
    }
  }

  clearForm() {
    this.error.set(null);
    this.draft.set({ username: '', password: '', password2: '', administrator: false });
  }

  async lookup() {
    const username = this.draft().username.trim();
    if (!username) return;

    this.error.set(null);
    this.loading.set(true);
    try {
      const u = await firstValueFrom(this.api.get(username));
      if (u) this.upsertUser(u);
    } catch (e: any) {
      this.error.set(e?.error?.message ?? e?.message ?? 'Failed to lookup user.');
    } finally {
      this.loading.set(false);
    }
  }

  async checkExists() {
    const username = this.draft().username.trim();
    if (!username) return;

    this.error.set(null);
    this.loading.set(true);
    try {
      const res = await firstValueFrom(this.api.exists(username));
      if (!res?.exists) this.error.set('User does not exist.');
      if (res?.exists) {
        // Existence confirmed; fetch safe record so the table stays accurate.
        await this.lookup();
      }
    } catch (e: any) {
      this.error.set(e?.error?.message ?? e?.message ?? 'Failed to check existence.');
    } finally {
      this.loading.set(false);
    }
  }

  async createUser() {
    const d = this.draft();
    const username = d.username.trim();
    if (!username) return;

    if (!d.password || d.password !== d.password2) {
      this.error.set('Passwords do not match.');
      return;
    }

    this.error.set(null);
    this.working.set(true);
    try {
      await firstValueFrom(this.api.create(username, d.password, d.administrator));
      await this.lookup(); // refresh safe fields into the table
      this.draft.set({ ...this.draft(), password: '', password2: '' });
    } catch (e: any) {
      this.error.set(e?.error?.message ?? e?.message ?? 'Failed to create user.');
    } finally {
      this.working.set(false);
    }
  }

  async resetPassword(username: string) {
    const d = this.draft();
    if (!d.password || d.password !== d.password2) {
      this.error.set('Passwords do not match.');
      return;
    }

    this.error.set(null);
    this.working.set(true);
    try {
      await firstValueFrom(this.api.updatePassword(username, d.password));
      this.draft.set({ ...this.draft(), password: '', password2: '' });
    } catch (e: any) {
      this.error.set(e?.error?.message ?? e?.message ?? 'Failed to update password.');
    } finally {
      this.working.set(false);
    }
  }

  updateDraft(patch: Partial<Draft>) {
    this.draft.set({ ...this.draft(), ...patch });
  }

  async setDisabled(username: string, disabled: boolean) {
    this.error.set(null);
    this.working.set(true);
    try {
      await firstValueFrom(this.api.disable(username, disabled));
      // optimistic UI update
      const existing = this.users().find((u) => u.username === username);
      if (existing) this.upsertUser({ ...existing, disabled });
      // If it wasn't in the table yet, pull full record:
      if (!existing) await this.lookup();
    } catch (e: any) {
      this.error.set(e?.error?.message ?? e?.message ?? 'Failed to update disabled state.');
    } finally {
      this.working.set(false);
    }
  }

  async delete(username: string) {
    const ok = confirm(`Delete local user "${username}"? This cannot be undone.`);
    if (!ok) return;

    this.error.set(null);
    this.working.set(true);
    try {
      await firstValueFrom(this.api.delete(username));
      this.users.set(this.users().filter((u) => u.username !== username));
    } catch (e: any) {
      this.error.set(e?.error?.message ?? e?.message ?? 'Failed to delete user.');
    } finally {
      this.working.set(false);
    }
  }

  selectRow(u: LocalUserSafe) {
    this.error.set(null);
    this.draft.set({
      username: u.username,
      password: '',
      password2: '',
      administrator: !!u.administrator,
    });
  }
}
