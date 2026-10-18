// Package proto defines the stele.v1 wire contract. Client frames are strict;
// server frames allow unknown members. Decoding is bounded and never consults
// state. Correlation is trusted only after the entire JSON frame passes syntax,
// Unicode and duplicate-name checks. Annotation bytes travel inside strings.
package proto
