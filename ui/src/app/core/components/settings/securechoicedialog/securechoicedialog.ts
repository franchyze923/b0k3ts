import { Component, Inject } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialogModule, MatDialogRef, MAT_DIALOG_DATA } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelectModule } from '@angular/material/select';

export type SecureChoiceDialogData = {
  bucketLabel: string;
  defaultSecure: boolean;
};

@Component({
  selector: 'app-secure-choice-dialog',
  standalone: true,
  imports: [MatDialogModule, MatButtonModule, MatFormFieldModule, MatSelectModule],
  templateUrl: 'securechoicedialog.html',
  styleUrls: ['./securechoicedialog.scss'],
})
export class SecureChoiceDialogComponent {
  value: boolean;

  constructor(
    private readonly ref: MatDialogRef<SecureChoiceDialogComponent, boolean | undefined>,
    @Inject(MAT_DIALOG_DATA) public readonly data: SecureChoiceDialogData,
  ) {
    this.value = data.defaultSecure;
  }

  cancel(): void {
    this.ref.close();
  }

  confirm(): void {
    this.ref.close(this.value);
  }
}
