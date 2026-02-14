import { Injectable } from '@angular/core';
import {
  HttpClient,
  HttpContext,
  HttpEvent,
  HttpEventType,
  HttpResponse,
} from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { SKIP_AUTH } from '../interceptor/auth-token-interceptor';
import { filter } from 'rxjs/operators';

type PresignDownloadRequest = {
  bucket: string;
  key: string;
  expires_seconds?: number;
  disposition?: 'attachment' | 'inline';
  filename?: string;
};

type PresignDownloadResponse = {
  url: string;
};

export type ObjectApiItem = {
  key: string;
  size: number;
  content_type: string;
};

type MultipartInitiateRequest = {
  bucket: string;
  key: string;
  content_type?: string;
};

type MultipartInitiateResponse = {
  bucket: string;
  key: string;
  upload_id: string;
};

type MultipartPresignPartRequest = {
  bucket: string;
  key: string;
  upload_id: string;
  part_number: number; // 1..10000
  expires_seconds?: number; // optional
};

type MultipartPresignPartResponse = {
  url: string;
};

type MultipartCompletedPart = {
  part_number: number;
  etag: string;
};

type MultipartCompleteRequest = {
  bucket: string;
  key: string;
  upload_id: string;
  parts: MultipartCompletedPart[];
};

type MultipartAbortRequest = {
  bucket: string;
  key: string;
  upload_id: string;
};

export type ObjectMoveRequest = {
  bucket: string;
  from_key?: string;
  to_key?: string;
  from_prefix?: string;
  to_prefix?: string;
  overwrite?: boolean; // optional; default false
};

@Injectable({ providedIn: 'root' })
export class ObjectStorageService {
  private readonly apiBase = '';

  constructor(private readonly http: HttpClient) {}

  async listObjects(params: { bucket: string; prefix?: string }): Promise<ObjectApiItem[]> {
    const url = `${this.apiBase}/api/v1/objects/list`;
    return await firstValueFrom(this.http.post<ObjectApiItem[]>(url, params));
  }

  private normalizeEtag(etag: string): string {
    const trimmed = etag.trim();
    if (trimmed.startsWith('"') && trimmed.endsWith('"') && trimmed.length >= 2) {
      return trimmed.slice(1, -1);
    }
    return trimmed;
  }

  private async multipartInitiate(
    params: MultipartInitiateRequest,
  ): Promise<MultipartInitiateResponse> {
    const url = `${this.apiBase}/api/v1/objects/multipart/initiate`;
    return await firstValueFrom(this.http.post<MultipartInitiateResponse>(url, params));
  }

  private async multipartPresignPart(
    params: MultipartPresignPartRequest,
  ): Promise<MultipartPresignPartResponse> {
    const url = `${this.apiBase}/api/v1/objects/multipart/presign_part`;
    return await firstValueFrom(this.http.post<MultipartPresignPartResponse>(url, params));
  }

  private async multipartComplete(params: MultipartCompleteRequest): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/multipart/complete`;
    await firstValueFrom(this.http.post<void>(url, params));
  }

  private async multipartAbort(params: MultipartAbortRequest): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/multipart/abort`;
    await firstValueFrom(this.http.post<void>(url, params));
  }

  async uploadObjectMultipart(params: {
    bucket: string;
    key: string;
    file: File;
    contentType?: string;
    partSizeBytes?: number; // default 8 MiB
    presignExpiresSeconds?: number; // default backend 900

    /**
     * Called as bytes are uploaded. Useful for progress bars.
     * Note: Some environments may not provide totalBytes reliably; we fall back to file.size.
     */
    onProgress?: (info: {
      percent: number; // 0..100
      uploadedBytes: number;
      totalBytes: number;
      partNumber: number; // 1..partCount
      partCount: number;
    }) => void;
  }): Promise<void> {
    const partSizeBytes = params.partSizeBytes ?? 8 * 1024 * 1024;

    const initiated = await this.multipartInitiate({
      bucket: params.bucket,
      key: params.key,
      content_type: params.contentType ?? params.file.type ?? 'application/octet-stream',
    });

    const uploadId = initiated.upload_id;

    try {
      const totalSize = params.file.size;
      const partCount = Math.max(1, Math.ceil(totalSize / partSizeBytes));

      if (partCount > 10000) {
        throw new Error(
          `File too large for chosen part size: ${partCount} parts (max 10000). Increase part size.`,
        );
      }

      const completedParts: MultipartCompletedPart[] = [];

      // Track aggregate progress across parts
      const baseUploadedByCompletedParts: number[] = [];
      let uploadedCompletedBytes = 0;

      for (let partNumber = 1; partNumber <= partCount; partNumber++) {
        const start = (partNumber - 1) * partSizeBytes;
        const end = Math.min(start + partSizeBytes, totalSize);
        const chunk = params.file.slice(start, end);
        const partSize = end - start;

        const presigned = await this.multipartPresignPart({
          bucket: params.bucket,
          key: params.key,
          upload_id: uploadId,
          part_number: partNumber,
          expires_seconds: params.presignExpiresSeconds,
        });

        let lastLoadedForPart = 0;

        let resp: HttpResponse<string>;
        try {
          resp = (await firstValueFrom(
            this.http
              .put(presigned.url, chunk, {
                observe: 'events',
                reportProgress: true,
                credentials: 'omit',
                responseType: 'text',
                context: new HttpContext().set(SKIP_AUTH, true),

                // Important: do NOT send custom headers to presigned URLs unless they were signed.
                // headers: { 'Content-Type': ... }  <-- remove
              })
              .pipe(
                filter((event: HttpEvent<string>) => {
                  if (event.type === HttpEventType.UploadProgress) {
                    const loaded = Math.min(event.loaded ?? 0, partSize);
                    lastLoadedForPart = loaded;

                    const uploadedBytes = Math.min(uploadedCompletedBytes + loaded, totalSize);
                    const totalBytes = totalSize;
                    const percent =
                      totalBytes > 0
                        ? Math.max(0, Math.min(100, (uploadedBytes / totalBytes) * 100))
                        : 100;

                    params.onProgress?.({
                      percent,
                      uploadedBytes,
                      totalBytes,
                      partNumber,
                      partCount,
                    });

                    return false; // keep waiting for final response
                  }

                  return event.type === HttpEventType.Response;
                }),
              ),
          )) as HttpResponse<string>;
        } catch (e) {
          const anyErr = e as any;
          const status = anyErr?.status;
          const statusText = anyErr?.statusText;
          const body = anyErr?.error;

          throw new Error(
            `Part ${partNumber} upload failed (${status ?? 'unknown'} ${statusText ?? ''}). ` +
              `Storage response: ${typeof body === 'string' ? body : '[no body]'}`,
          );
        }

        // Mark this part as fully uploaded in our aggregate tracking
        uploadedCompletedBytes += partSize;
        baseUploadedByCompletedParts.push(partSize);

        // Emit a "part finished" progress tick (helps UI snap to clean boundaries)
        const uploadedBytes = Math.min(uploadedCompletedBytes, totalSize);
        const percent =
          totalSize > 0 ? Math.max(0, Math.min(100, (uploadedBytes / totalSize) * 100)) : 100;

        params.onProgress?.({
          percent,
          uploadedBytes,
          totalBytes: totalSize,
          partNumber,
          partCount,
        });

        const etagHeader = resp.headers.get('ETag') ?? resp.headers.get('etag');
        if (!etagHeader) {
          throw new Error(
            `Missing ETag for part ${partNumber}. Ensure storage exposes ETag header via CORS.`,
          );
        }

        completedParts.push({
          part_number: partNumber,
          etag: this.normalizeEtag(etagHeader),
        });
      }

      await this.multipartComplete({
        bucket: params.bucket,
        key: params.key,
        upload_id: uploadId,
        parts: completedParts,
      });

      // Ensure final 100% callback (in case rounding left you at 99.9%)
      params.onProgress?.({
        percent: 100,
        uploadedBytes: totalSize,
        totalBytes: totalSize,
        partNumber: partCount,
        partCount,
      });
    } catch (err) {
      try {
        await this.multipartAbort({ bucket: params.bucket, key: params.key, upload_id: uploadId });
      } catch {
        // ignore
      }
      throw err;
    }
  }

  async presignDownload(params: PresignDownloadRequest): Promise<PresignDownloadResponse> {
    const url = `${this.apiBase}/api/v1/objects/presign-download`;
    return await firstValueFrom(this.http.post<PresignDownloadResponse>(url, params));
  }

  async downloadObjectNative(params: {
    bucket: string;
    key: string;
    filename?: string;
    disposition?: 'attachment' | 'inline';
    expiresSeconds?: number;
    openInNewTab?: boolean;
  }): Promise<void> {
    const res = await this.presignDownload({
      bucket: params.bucket,
      key: params.key,
      filename: params.filename,
      disposition: params.disposition ?? 'attachment',
      expires_seconds: params.expiresSeconds ?? 900,
    });

    if (params.openInNewTab) {
      window.open(res.url, '_blank', 'noopener');
      return;
    }

    window.location.assign(res.url);
  }

  async deleteObject(params: { bucket: string; filename: string }): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/delete`;
    await firstValueFrom(this.http.post<void>(url, params));
  }

  async moveObjects(params: ObjectMoveRequest): Promise<void> {
    const url = `${this.apiBase}/api/v1/objects/move`;
    await firstValueFrom(this.http.post<void>(url, params));
  }

  async moveObject(params: {
    bucket: string;
    fromKey: string;
    toKey: string;
    overwrite?: boolean;
  }): Promise<void> {
    await this.moveObjects({
      bucket: params.bucket,
      from_key: params.fromKey,
      to_key: params.toKey,
      overwrite: params.overwrite,
    });
  }

  async movePrefix(params: {
    bucket: string;
    fromPrefix: string;
    toPrefix: string;
    overwrite?: boolean;
  }): Promise<void> {
    await this.moveObjects({
      bucket: params.bucket,
      from_prefix: params.fromPrefix,
      to_prefix: params.toPrefix,
      overwrite: params.overwrite,
    });
  }

  async moveObjectByPrefix(params: {
    bucket: string;
    sourceKey: string;
    destinationPrefix: string;
    overwrite?: boolean;
  }): Promise<{
    destinationKey: string;
  }> {
    const fileName = params.sourceKey.split('/').filter(Boolean).pop() ?? params.sourceKey;

    // Avoid regex (linear-time scans only)
    const p = params.destinationPrefix.trim();
    let normalizedPrefix = '';
    if (p.length !== 0) {
      let start = 0;
      let end = p.length;

      while (start < end && p.charCodeAt(start) === 47) start++; // '/'
      while (end > start && p.charCodeAt(end - 1) === 47) end--; // '/'

      const core = p.slice(start, end);
      normalizedPrefix = core === '' ? '' : core + '/';
    }

    const destinationKey = `${normalizedPrefix}${fileName}`;

    await this.moveObject({
      bucket: params.bucket,
      fromKey: params.sourceKey,
      toKey: destinationKey,
      overwrite: params.overwrite,
    });

    return { destinationKey };
  }
}
