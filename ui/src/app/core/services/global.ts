import { Injectable } from '@angular/core';
import { BehaviorSubject } from 'rxjs';

@Injectable({
  providedIn: 'root',
})
export class GlobalService {
  readonly textSource = new BehaviorSubject<string>('B0K3TS');
  currentTitle = this.textSource.asObservable();

  updateTitle(newText: string) {
    this.textSource.next(newText);
  }
}
