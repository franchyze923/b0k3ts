import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type ObjectApiItem = {
  key: string;
  size: number; // backend: int64
  content_type: string;
};

@Injectable({ providedIn: 'root' })
export class ObjectStorageService {
  private readonly apiBase = ''; // keep '' for same-origin; set if needed

  constructor(private readonly http: HttpClient) {}

  async listObjects(params: { bucket: string; prefix?: string }): Promise<ObjectApiItem[]> {
    const url = `${this.apiBase}/api/v1/objects/list`;
    return await firstValueFrom(this.http.post<ObjectApiItem[]>(url, params));
  }

  /**
   * Uploads an object by sending:
   * - name (we use `key` as the "name" to preserve paths like `reports/2026.pdf`)
   * - bytes (number[] from Uint8Array)
   */
  async uploadObject(params: { bucket: string; key: string; bytes: number[]; contentType?: string }): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/upload`;
    await firstValueFrom(
      this.http.post<void>(url, {
        bucket: params.bucket,
        name: params.key,
        bytes: params.bytes,
        content_type: params.contentType,
      }),
    );
  }

  /**
   * Downloads raw bytes as ArrayBuffer.
   * If your backend returns JSON `number[]` instead of raw bytes, tell me and I’ll adjust this.
   */
  async downloadObject(params: { bucket: string; key: string }): Promise<ArrayBuffer> {
    const url = `${this.apiBase}/api/v1/objects/download`;
    return await firstValueFrom(
      this.http.post(url, { bucket: params.bucket, key: params.key }, { responseType: 'arraybuffer' }),
    );
  }

  async deleteObject(params: { bucket: string; key: string }): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/delete`;
    await firstValueFrom(this.http.post<void>(url, params));
  }

  async moveObjectByPrefix(params: { bucket: string; sourceKey: string; destinationPrefix: string }): Promise<{
    destinationKey: string;
  }> {
    const fileName = params.sourceKey.split('/').filter(Boolean).pop() ?? params.sourceKey;
    const normalizedPrefix =
      params.destinationPrefix.trim().length === 0
        ? ''
        : params.destinationPrefix.trim().replace(/^\/+/, '').replace(/\/+$/, '') + '/';

    const destinationKey = `${normalizedPrefix}${fileName}`;

    // download -> upload -> delete (uses only the allowed APIs)
    const bytesBuffer = await this.downloadObject({ bucket: params.bucket, key: params.sourceKey });
    const bytes = Array.from(new Uint8Array(bytesBuffer));

    await this.uploadObject({
      bucket: params.bucket,
      key: destinationKey,
      bytes,
    });

    await this.deleteObject({
      bucket: params.bucket,
      key: params.sourceKey,
    });

    return { destinationKey };
  }
}
