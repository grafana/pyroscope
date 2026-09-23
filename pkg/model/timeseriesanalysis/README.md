# Time-series anomaly scoring playground

Open `index.html` directly in a browser. No build, dependencies, or backend required.

Alternatively, from the repository root:

```sh
python3 -m http.server 8080 --directory pkg/model/timeseriesanalysis
```

Visit http://localhost:8080.

Ten graphs compare stable and noisy services, spikes, sustained changes, drops,
zero baselines, and absolute impact. Adjust the score and relative-change gates
(defaults: 3.5 and 25%) to see which first samples qualify.

The scoring formula matches `pkg/model/timeseriesanalysis/analysis.go`: median
baseline, MAD scaled by 1.4826, and a floor of 1% of the absolute baseline or 1
unit. Peak scores use the same pre-event variability as detection.

This intentionally illustrates fixed event windows, not the complete production
detector. It does not reproduce gap handling, flatness filtering, recovery,
confirmation, or automatic event segmentation. A passing candidate is not a
promise that the full detector will emit that exact window. Scores are neither
probabilities nor operational severity ratings.
