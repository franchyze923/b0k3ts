import { Component, Inject } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatSnackBar } from '@angular/material/snack-bar';

type AwsCreds = {
  AWS_ACCESS_KEY_ID?: string;
  AWS_SECRET_ACCESS_KEY?: string;
};

@Component({
  standalone: true,
  imports: [MatDialogModule, MatButtonModule, MatIconModule, MatTooltipModule],
  templateUrl: './cephcredssnack.html',
  styleUrl: './cephcredssnack.scss',
})
export class CephCredsDialogComponent {
  get accessKeyId(): string {
    return this.data.credentials?.AWS_ACCESS_KEY_ID ?? '';
  }

  get secretKey(): string {
    return this.data.credentials?.AWS_SECRET_ACCESS_KEY ?? '';
  }

  constructor(
    private readonly ref: MatDialogRef<CephCredsDialogComponent>,
    @Inject(MAT_DIALOG_DATA)
    public readonly data: { title: string; credentials: AwsCreds },
    private readonly snack: MatSnackBar,
  ) {}

  close(): void {
    this.ref.close();
  }

  async copy(text: string, toast: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      this.snack.open(toast, 'Dismiss', { duration: 1500 });
    } catch {
      this.snack.open('Copy failed (clipboard not available)', 'Dismiss', { duration: 2500 });
    }
  }

  async copyJson(): Promise<void> {
    const json = JSON.stringify(
      {
        AWS_ACCESS_KEY_ID: this.accessKeyId,
        AWS_SECRET_ACCESS_KEY: this.secretKey,
      },
      null,
      2,
    );

    await this.copy(json, 'Credentials JSON copied');
  }
}
