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
  templateUrl: './move-prefix-dialog.html',
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
