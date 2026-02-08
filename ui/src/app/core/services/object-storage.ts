import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type ObjectApiItem = {
  key: string;
  size: number;
  content_type: string;
};

@Injectable({ providedIn: 'root' })
export class ObjectStorageService {
  private readonly apiBase = '';

  constructor(private readonly http: HttpClient) {}

  async listObjects(params: { bucket: string; prefix?: string }): Promise<ObjectApiItem[]> {
    const url = `${this.apiBase}/api/v1/objects/list`;
    return await firstValueFrom(this.http.post<ObjectApiItem[]>(url, params));
  }

  async uploadObject(params: {
    bucket: string;
    key: string;
    bytes: number[];
    contentType?: string;
  }): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/upload`;

    const fileName = params.key.split('/').filter(Boolean).pop() ?? params.key;
    const blob = new Blob([new Uint8Array(params.bytes)], {
      type: params.contentType ?? 'application/octet-stream',
    });

    const form = new FormData();
    form.append('bucket', params.bucket);
    form.append('name', params.key);
    form.append('file', blob, fileName);

    if (params.contentType) form.append('content_type', params.contentType);

    await firstValueFrom(this.http.post<void>(url, form));
  }

  async downloadObject(params: { bucket: string; filename: string }): Promise<Blob> {
    const url = `${this.apiBase}/api/v1/objects/download`;
    return await firstValueFrom(
      this.http.post(
        url,
        { bucket: params.bucket, filename: params.filename },
        { responseType: 'blob' },
      ),
    );
  }

  async deleteObject(params: { bucket: string; filename: string }): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/delete`;
    await firstValueFrom(this.http.post<void>(url, params));
  }

  async moveObjectByPrefix(params: {
    bucket: string;
    sourceKey: string;
    destinationPrefix: string;
  }): Promise<{
    destinationKey: string;
  }> {
    const fileName = params.sourceKey.split('/').filter(Boolean).pop() ?? params.sourceKey;
    const normalizedPrefix =
      params.destinationPrefix.trim().length === 0
        ? ''
        : params.destinationPrefix.trim().replace(/^\/+/, '').replace(/\/+$/, '') + '/';

    const destinationKey = `${normalizedPrefix}${fileName}`;

    const blob = await this.downloadObject({ bucket: params.bucket, filename: params.sourceKey });

    const blobBytes = await blob.arrayBuffer();
    const bytes: number[] = Array.from(new Uint8Array(blobBytes));

    await this.uploadObject({
      bucket: params.bucket,
      key: destinationKey,
      bytes,
    });

    await this.deleteObject({
      bucket: params.bucket,
      filename: params.sourceKey,
    });

    return { destinationKey };
  }
}
