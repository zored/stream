use DynaLoader;
my $lib = DynaLoader::dl_load_file($ARGV[0], 0) or die DynaLoader::dl_error();
my $sym = DynaLoader::dl_find_symbol($lib, "streamd_main") or die DynaLoader::dl_error();
DynaLoader::dl_install_xsub("main::streamd_main", $sym)->();
