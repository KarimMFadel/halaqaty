/// Retry intervals for one logical chat send after its initial failure.
const chatRetryDelays = [
  Duration(seconds: 1),
  Duration(seconds: 2),
  Duration(seconds: 4),
];
