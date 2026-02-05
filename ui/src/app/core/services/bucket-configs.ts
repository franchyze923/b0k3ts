import { Injectable } from '@angular/core';

export type BucketConfig = {
  bucket_id: string;
  endpoint: string;
  access_key_id: string;
  secret_access_key: string;
  secure: boolean;
  bucket_name: string;
  location: string;
};

@Injectable({ providedIn: 'root' })
export class BucketConfigsService {
  private readonly storageKey = 'bucket.configs.v1';

  list(): BucketConfig[] {
    return this.readAll();
  }

  add(cfg: BucketConfig): BucketConfig[] {
    const all = this.readAll();

    if (all.some((b) => b.bucket_id === cfg.bucket_id)) {
      throw new Error(`BucketId "${cfg.bucket_id}" already exists.`);
    }

    const next = [...all, cfg];
    this.writeAll(next);
    return next;
  }

  update(bucketId: string, patch: BucketConfig): BucketConfig[] {
    const all = this.readAll();
    const idx = all.findIndex((b) => b.bucket_id === bucketId);
    if (idx === -1) throw new Error(`BucketId "${bucketId}" not found.`);

    // If user changes bucket_id, ensure uniqueness
    if (patch.bucket_id !== bucketId && all.some((b) => b.bucket_id === patch.bucket_id)) {
      throw new Error(`BucketId "${patch.bucket_id}" already exists.`);
    }

    const next = all.map((b) => (b.bucket_id === bucketId ? patch : b));
    this.writeAll(next);
    return next;
  }

  remove(bucketId: string): BucketConfig[] {
    const all = this.readAll();
    const next = all.filter((b) => b.bucket_id !== bucketId);
    this.writeAll(next);
    return next;
  }

  private readAll(): BucketConfig[] {
    const raw = localStorage.getItem(this.storageKey);
    if (!raw) return [];

    try {
      const parsed = JSON.parse(raw) as unknown;
      if (!Array.isArray(parsed)) return [];
      return parsed as BucketConfig[];
    } catch {
      return [];
    }
  }

  private writeAll(configs: BucketConfig[]): void {
    localStorage.setItem(this.storageKey, JSON.stringify(configs));
  }
}
