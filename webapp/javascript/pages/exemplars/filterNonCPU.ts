/**
 * filterNonCPU filters out apps that are not cpu
 * it DOES not filter apps that can't be identified
 * Notice that the heuristic here is weak and should be updated when more info is available (such as units)
 */
export function filterNonCPU(appName: string): boolean {
  const suffix = appName.split('.').pop();
  if (!suffix) {
    return true;
  }

  // Golang
  if (suffix.includes('alloc_objects')) {
    return false;
  }

  if (suffix.includes('alloc_space')) {
    return false;
  }

  if (suffix.includes('goroutines')) {
    return false;
  }

  if (suffix.includes('inuse_objects')) {
    return false;
  }

  if (suffix.includes('inuse_space')) {
    return false;
  }

  if (suffix.includes('mutex_count')) {
    return false;
  }

  if (suffix.includes('mutex_duration')) {
    return false;
  }

  // Java
  if (suffix.includes('alloc_in_new_tlab_bytes')) {
    return false;
  }

  if (suffix.includes('alloc_in_new_tlab_objects')) {
    return false;
  }

  if (suffix.includes('lock_count')) {
    return false;
  }

  if (suffix.includes('lock_duration')) {
    return false;
  }

  return true;
}
