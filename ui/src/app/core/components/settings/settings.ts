import { Component, inject, OnInit } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { GlobalService } from '../../services/global';

@Component({
  selector: 'app-settings',
  imports: [RouterOutlet],
  templateUrl: './settings.html',
  styleUrl: './settings.scss',
})
export class Settings implements OnInit {
  private readonly global = inject(GlobalService);

  constructor() {}
  ngOnInit(): void {
    queueMicrotask(() => this.global.updateTitle('Settings'));
  }
}
