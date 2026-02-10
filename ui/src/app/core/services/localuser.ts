import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

export type LocalUserSafe = {
  username: string;
  created_at: any;
  updated_at: any;
  disabled: boolean;
  email: string;
  groups: string[];
  administrator: boolean;
};

@Injectable({ providedIn: 'root' })
export class LocalUsersService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api/v1/local/users';

  exists(username: string): Observable<{ exists: boolean }> {
    return this.http.post<{ exists: boolean }>(`${this.base}/exists`, { username });
  }

  create(username: string, password: string, administrator: boolean): Observable<unknown> {
    return this.http.post(`${this.base}/create`, { username, password, administrator });
  }

  // (Optional for later): ensure(username, password, administrator) → POST /ensure

  get(username: string): Observable<LocalUserSafe> {
    return this.http.get<LocalUserSafe>(`${this.base}/${encodeURIComponent(username)}`);
  }

  disable(username: string, disabled: boolean): Observable<unknown> {
    return this.http.post(`${this.base}/disable`, { username, disabled });
  }

  delete(username: string): Observable<unknown> {
    return this.http.post(`${this.base}/delete`, { username });
  }

  updatePassword(username: string, newPassword: string): Observable<unknown> {
    return this.http.post(`${this.base}/update_password`, {
      username,
      new_password: newPassword,
    });
  }
}
