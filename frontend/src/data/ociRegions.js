// OCI region codes → [lat, lng] coordinates.
// For multi-AD regions we pin the city center.

const ociRegions = {
  // North America
  'us-phoenix-1':       [33.4484, -112.0740],
  'us-ashburn-1':       [39.0438,  -77.4874],
  'us-sanjose-1':       [37.3382, -121.8863],
  'us-chicago-1':       [41.8781,  -87.6298],
  'us-saltlake-1':      [40.7608, -111.8910],
  'us-langley-1':       [38.9498,  -77.3623],
  'us-luke-1':          [33.5360, -112.3825],
  'ca-montreal-1':      [45.5017,  -73.5673],
  'ca-toronto-1':       [43.6532,  -79.3832],
  'mx-queretaro-1':     [20.5888, -100.3899],
  'mx-monterrey-1':     [25.6866, -100.3161],

  // South America
  'sa-saopaulo-1':      [-23.5505, -46.6333],
  'sa-vinhedo-1':       [-23.0300, -47.0000],
  'sa-santiago-1':      [-33.4489, -70.6693],
  'sa-bogota-1':        [4.7110,  -74.0721],

  // Europe
  'eu-frankfurt-1':     [50.1109,   8.6821],
  'eu-amsterdam-1':     [52.3676,   4.9041],
  'eu-zurich-1':        [47.3769,   8.5417],
  'eu-london-1':        [51.5074,  -0.1278],
  'eu-madrid-1':        [40.4168,  -3.7038],
  'eu-marseille-1':     [43.2965,   5.3698],
  'eu-milan-1':         [45.4642,   9.1900],
  'eu-stockholm-1':     [59.3293,  18.0686],
  'eu-paris-1':         [48.8566,   2.3522],
  'eu-cardiff-1':       [51.4816,  -3.1791],

  // UK
  'uk-london-1':        [51.5074,  -0.1278],
  'uk-cardiff-1':       [51.4816,  -3.1791],

  // Middle East & Africa
  'me-jeddah-1':        [21.5433,  39.1728],
  'me-dubai-1':         [25.2048,  55.2708],
  'me-abudhabi-1':      [24.4539,  54.3773],
  'me-dcc-muscat-1':    [23.5880,  58.3829],
  'sa-riyadh-1':        [24.7136,  46.6753],
  'af-johannesburg-1':  [-26.2041, 28.0473],

  // Asia Pacific
  'ap-tokyo-1':         [35.6762, 139.6503],
  'ap-osaka-1':         [34.6937, 135.5023],
  'ap-seoul-1':         [37.5665, 126.9780],
  'ap-chuncheon-1':     [37.8813, 127.7300],
  'ap-mumbai-1':        [19.0760,  72.8777],
  'ap-hyderabad-1':     [17.3850,  78.4867],
  'ap-singapore-1':     [1.3521,  103.8198],
  'ap-sydney-1':        [-33.8688, 151.2093],
  'ap-melbourne-1':     [-37.8136, 144.9631],
  'ap-manila-1':        [14.5995, 120.9842],

  // Australia
  'au-sydney-1':        [-33.8688, 151.2093],
  'au-melbourne-1':     [-37.8136, 144.9631],

  // Japan
  'jp-tokyo-1':         [35.6762, 139.6503],

  // Additional region aliases
  'us-sanjose-2':       [37.3382, -121.8863],
  'ap-chiyoda-1':       [35.6895, 139.7670],
  'ap-ibaraki-1':       [36.3447, 140.4496],
}

export default ociRegions
