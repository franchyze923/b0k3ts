import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { GlobalService } from '../../services/global';

@Component({
  selector: 'app-settings',
  imports: [RouterOutlet],
  templateUrl: './settings.html',
  styleUrl: './settings.scss',
})
export class Settings {
  private readonly global = inject(GlobalService);

  constructor() {
    this.global.updateTitle('Settings');
  }
}
