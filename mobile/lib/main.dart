import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'app.dart';
import 'stores/app_state.dart';

void main() {
  runApp(
    ChangeNotifierProvider(
      create: (_) => AppState()..init(),
      child: const ShellSyncApp(),
    ),
  );
}
