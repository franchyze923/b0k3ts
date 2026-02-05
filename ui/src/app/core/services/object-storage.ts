import { Injectable } from '@angular/core';

@Injectable({ providedIn: 'root' })
export class ObjectStorageService {
  async copyObject(params: { bucket: string; sourceKey: string; destinationKey: string }): Promise<void> {
    // TODO: integrate backend call
    console.log('[ObjectStorage] copyObject', params);
  }

  async deleteObject(params: { bucket: string; key: string }): Promise<void> {
    // TODO: integrate backend call
    console.log('[ObjectStorage] deleteObject', params);
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

    // Copy then delete (classic object-store "move")
    await this.copyObject({
      bucket: params.bucket,
      sourceKey: params.sourceKey,
      destinationKey,
    });

    await this.deleteObject({
      bucket: params.bucket,
      key: params.sourceKey,
    });

    return { destinationKey };
  }
}
