import { LRUCache } from 'lru-cache'



type ImageCacheEntry = Readonly<{
  blob: Blob;
  objectUrl: string;
}>;


class ImageCacheManager {

  private readonly cache: LRUCache<string, ImageCacheEntry>

  private readonly inflight = new Map<string, Promise<string>>();


  constructor() {
    this.cache = new LRUCache<string, ImageCacheEntry>({

      max: 100,

      maxSize: 1024 * 1024 * 1024, // 1GB 

      sizeCalculation: (value: ImageCacheEntry, _key: string): number => {
        return value.blob.size;
      },

      dispose: (value: ImageCacheEntry, key: string, reason: LRUCache.DisposeReason) => {

        URL.revokeObjectURL(value.objectUrl);
      },
    });
  }

  async get(key: string, fetcher: () => Promise<{blob : Blob | null, err : string}>, fallbackUrl: string): Promise<string> {
    const cached = this.cache.get(key);
    
    if (cached) return cached.objectUrl;

    if (this.inflight.has(key)) {
      return this.inflight.get(key)!;
    }
    console.log("miss: " + key)

    const promise = (async () => {
      const {blob, err}= await fetcher();
      if (err != "" || !blob){
        console.log("error fetching image: " + err)
        return fallbackUrl
      }

      const objectUrl = URL.createObjectURL(blob)
      this.cache.set(key, { blob, objectUrl })
      this.inflight.delete(key)
      return objectUrl
    })()

    this.inflight.set(key, promise)
    return promise
  }


  has(key: string): boolean {
    return this.cache.has(key)
  }

  delete(key: string): boolean {
    return this.cache.delete(key)
  }

  clear() {
    this.cache.clear()
  }
}



export const imageCache = new ImageCacheManager()