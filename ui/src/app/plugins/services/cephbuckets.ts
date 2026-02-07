import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type CephBucketCredentials = {
  bucket_name?: string;
  access_key_id?: string;
  secret_access_key?: string;
  endpoint?: string;
  secure?: boolean;
  region?: string;
  location?: string;
};

@Injectable({ providedIn: 'root' })
export class CephBucketsService {
  private readonly apiBase = ''; // keep '' for same-origin; proxy handles /api/v1

  constructor(private readonly http: HttpClient) {}

  async listBuckets(): Promise<string[]> {
    const url = `${this.apiBase}/api/v1/buckets/list`;

    // Backend might return string[] OR { buckets: string[] }
    const res = await firstValueFrom(this.http.get<unknown>(url));

    if (Array.isArray(res) && res.every((x) => typeof x === 'string')) return res;

    if (
      res &&
      typeof res === 'object' &&
      'buckets' in res &&
      Array.isArray((res as { buckets: unknown }).buckets)
    ) {
      return (res as { buckets: unknown[] }).buckets.filter((x): x is string => typeof x === 'string');
    }

    return [];
  }

  async createBucket(bucketName: string): Promise<CephBucketCredentials> {
    const url = `${this.apiBase}/api/v1/buckets/create`;

    // If your backend expects a different field name, tell me (e.g. { name }).
    return await firstValueFrom(
      this.http.post<CephBucketCredentials>(url, {
        bucket_name: bucketName,
      }),
    );
  }

  async deleteBucket(bucketName: string): Promise<void> {
    const url = `${this.apiBase}/api/v1/buckets/delete`;

    // If your backend expects a different field name, tell me (e.g. { name }).
    await firstValueFrom(
      this.http.post<void>(url, {
        bucket_name: bucketName,
      }),
    );
  }
}
