import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type BucketConfig = {
  bucket_id: string;
  endpoint: string;
  access_key_id: string;
  secret_access_key: string;
  secure: boolean;
  bucket_name: string;
  location: string;

  authorized_users: string[]; // Email
  authorized_groups: string[];
};

@Injectable({ providedIn: 'root' })
export class BucketConfigsService {
  private readonly apiBase = ''; // keep '' for same-origin; set if needed

  constructor(private readonly http: HttpClient) {}

  async listConnections(): Promise<BucketConfig[]> {
    const url = `${this.apiBase}/api/v1/buckets/list_connections`;

    const result = await firstValueFrom(this.http.get<BucketConfig[] | null>(url));

    // Backend may return null -> normalize to empty list
    return result ?? [];
  }

  /**
   * Sends the bucket connection to backend (no frontend persistence).
   * Treating as "add or update" (upsert) from the UI perspective.
   */
  async addConnection(cfg: BucketConfig): Promise<void> {
    const url = `${this.apiBase}/api/v1/buckets/add_connection`;
    await firstValueFrom(this.http.post<void>(url, cfg));
  }

  async deleteConnection(bucketId: string): Promise<void> {
    const url = `${this.apiBase}/api/v1/buckets/delete_connection`;
    await firstValueFrom(
      this.http.post<void>(url, {
        bucket_id: bucketId,
      }),
    );
  }
}
