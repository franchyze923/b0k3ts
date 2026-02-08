import { Component, computed, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { MatTableModule } from '@angular/material/table';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';

import { GlobalService } from '../../../services/global';
import { KubernetesKubeconfigsService } from '../../../services/kuberneteskubeconfigs';

@Component({
  selector: 'app-kubernetes',
  imports: [
    CommonModule,
    FormsModule,
    MatCardModule,
    MatButtonModule,
    MatIconModule,
    MatFormFieldModule,
    MatInputModule,
    MatSnackBarModule,
    MatTableModule,
    MatProgressSpinnerModule,
  ],
  templateUrl: './kubernetes.html',
  styleUrl: './kubernetes.scss',
})
export class Kubernetes {
  private readonly global = inject(GlobalService);
  private readonly kubeconfigs = inject(KubernetesKubeconfigsService);
  private readonly snack = inject(MatSnackBar);

  readonly saving = signal(false);

  readonly name = signal<string>('dev');
  readonly selectedFile = signal<File | null>(null);

  readonly kubeconfigNames = signal<string[]>([]);
  readonly listLoading = signal(false);
  readonly deletingName = signal<string | null>(null);

  readonly displayedColumns = ['name', 'actions'] as const;

  readonly canUpload = computed(() => {
    if (this.saving()) return false;
    if (!this.selectedFile()) return false;
    if (this.name().trim().length === 0) return false;
    return true;
  });

  constructor() {
    this.global.updateTitle('Settings · Kubernetes');
    void this.refreshKubeconfigs();
  }

  onFileSelected(e: Event): void {
    const input = e.target as HTMLInputElement | null;
    const file = input?.files?.[0] ?? null;
    this.selectedFile.set(file);
  }

  clearFile(fileInput: HTMLInputElement): void {
    fileInput.value = '';
    this.selectedFile.set(null);
  }

  async refreshKubeconfigs(): Promise<void> {
    try {
      this.listLoading.set(true);
      const names = await this.kubeconfigs.listKubeconfigs();
      this.kubeconfigNames.set(names);
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to load kubeconfigs';
      this.snack.open(msg, 'Dismiss', { duration: 5000 });
    } finally {
      this.listLoading.set(false);
    }
  }

  async deleteKubeconfig(name: string): Promise<void> {
    if (this.deletingName()) return;

    const ok = window.confirm(`Delete kubeconfig "${name}"?`);
    if (!ok) return;

    try {
      this.deletingName.set(name);
      await this.kubeconfigs.deleteKubeconfig(name);
      this.snack.open(`Kubeconfig "${name}" deleted`, 'Dismiss', { duration: 2500 });

      // Optimistic update + safety refresh (in case backend normalized names)
      this.kubeconfigNames.update((xs) => xs.filter((x) => x !== name));
      await this.refreshKubeconfigs();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Delete failed';
      this.snack.open(msg, 'Dismiss', { duration: 5000 });
    } finally {
      this.deletingName.set(null);
    }
  }

  async upload(): Promise<void> {
    const name = this.name().trim();
    const file = this.selectedFile();

    const err = this.validate(name, file);
    if (err) {
      this.snack.open(err, 'Dismiss', { duration: 4000 });
      return;
    }

    try {
      this.saving.set(true);
      await this.kubeconfigs.uploadKubeconfig(name, file!);
      this.snack.open(`Kubeconfig "${name}" uploaded`, 'Dismiss', { duration: 2500 });
      await this.refreshKubeconfigs();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Upload failed';
      this.snack.open(msg, 'Dismiss', { duration: 5000 });
    } finally {
      this.saving.set(false);
    }
  }

  private validate(name: string, file: File | null): string | null {
    if (!name) return 'Please enter a name (e.g. dev)';
    if (!/^[a-zA-Z0-9_.-]+$/.test(name)) {
      return 'Name can only contain letters, numbers, dot, underscore, and dash';
    }
    if (!file) return 'Please choose a kubeconfig file to upload';
    return null;
  }
}
