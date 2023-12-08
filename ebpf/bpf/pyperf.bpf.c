#include "pyperf.common.h"

#define PYPERF_TEMPLATE_SECTION "perf_event"
#define PYPERF_TEMPLATE_SUFFIX perf
#define PYPERF_TEMPLATE_COLLECT_FLAGS FLAG_IS_CPU

#include "pyperf.progs.template.h"

#undef PYPERF_TEMPLATE_SECTION
#undef PYPERF_TEMPLATE_SUFFIX
#undef PYPERF_TEMPLATE_COLLECT_FLAGS

#define PYPERF_TEMPLATE_SECTION "uprobe/mem"
#define PYPERF_TEMPLATE_SUFFIX mem
#define PYPERF_TEMPLATE_COLLECT_FLAGS FLAG_IS_MEM

#include "pyperf.progs.template.h"


char _license[] SEC("license") = "GPL";
