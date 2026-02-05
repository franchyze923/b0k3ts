import { Component, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';

export type MovePrefixDialogData = {
  sourceKey: string;
  currentPrefix: string;
};

export type MovePrefixDialogResult = {
  destinationPrefix: string;
} | null;

@Component({
  selector: 'app-move-prefix-dialog',
  standalone: true,
  imports: [FormsModule, MatDialogModule, MatButtonModule, MatFormFieldModule, MatInputModule],
  template: `
    <h2 mat-dialog-title>Move object</h2>

    <div mat-dialog-content>
      <p><strong>Object:</strong> <code>{{ data.sourceKey }}</code></p>

      <mat-form-field appearance="outline" style="width: 100%;">
        <mat-label>Destination prefix (folder)</mat-label>
        <input
          matInput
          [(ngModel)]="destinationPrefix"
          placeholder="e.g. archive/2026/"
          autocomplete="off"
        />
        <mat-hint>Leave empty to move to the bucket root.</mat-hint>
      </mat-form-field>
    </div>

    <div mat-dialog-actions align="end">
      <button mat-button type="button" (click)="close(null)">Cancel</button>
      <button mat-flat-button color="primary" type="button" (click)="submit()">Move</button>
    </div>
  `,
})
export class MovePrefixDialog {
  readonly data = inject<MovePrefixDialogData>(MAT_DIALOG_DATA);
  private readonly dialogRef = inject(MatDialogRef<MovePrefixDialog, MovePrefixDialogResult>);

  destinationPrefix = this.data.currentPrefix;

  close(result: MovePrefixDialogResult): void {
    this.dialogRef.close(result);
  }

  submit(): void {
    this.close({ destinationPrefix: this.destinationPrefix ?? '' });
  }
}
