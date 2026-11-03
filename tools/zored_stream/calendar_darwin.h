char *cal_items(double from, double to, char **err);
void *cal_canvas_new(double w, double h, double scale, int rotate);
void cal_canvas_glyph(void *c, const char *text, double cx, double cy, double size, double alpha);
void cal_canvas_ring(void *c, double cx, double cy, double r, double width, unsigned rgb);
void cal_canvas_text(void *c, const char *text, double x, double y, double w, double h, double size, unsigned rgb, int center);
void *cal_canvas_png(void *c, int *len);
char *cal_set_done(const char *id, int done);
